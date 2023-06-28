package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/utils/dbutil"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type ExternalServiceID null.Int

func (i ExternalServiceID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

func (i *ExternalServiceID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

func (i ExternalServiceID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

func (i *ExternalServiceID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

func init() {
	goflow.RegisterExternalServiceServiceFactory(externalServiceServiceFactory)
}

func externalServiceServiceFactory(c *runtime.Config) engine.ExternalServiceServiceFactory {
	return func(sessiion flows.Session, externalService *flows.ExternalService) (flows.ExternalServiceService, error) {
		return externalService.Asset().(*ExternalService).AsService(c, externalService)
	}
}

type ExternalService struct {
	e struct {
		ID     ExternalServiceID          `json:"id,omitempty"`
		UUID   assets.ExternalServiceUUID `json:"uuid,omitempty"`
		OrgID  OrgID                      `json:"org_id,omitempty"`
		Type   string                     `json:"external_service_type,omitempty"`
		Name   string                     `json:"name,omitempty"`
		Config map[string]string          `json:"config,omitempty"`
	}
}

func (e *ExternalService) ID() ExternalServiceID { return e.e.ID }

func (e *ExternalService) UUID() assets.ExternalServiceUUID { return e.e.UUID }

func (e *ExternalService) OrgID() OrgID { return e.e.OrgID }

func (e *ExternalService) Name() string { return e.e.Name }

func (e *ExternalService) Type() string { return e.e.Type }

func (e *ExternalService) Config(key string) string { return e.e.Config[key] }

func (e *ExternalService) Reference() *assets.ExternalServiceReference {
	return assets.NewExternalServiceReference(e.e.UUID, e.e.Name)
}

func (e *ExternalService) AsService(cfg *runtime.Config, externalService *flows.ExternalService) (ExternalServiceService, error) {
	httpClient, httpRetries, _ := goflow.HTTP(cfg)

	initFunc := externalServiceServices[e.Type()]
	if initFunc != nil {
		return initFunc(cfg, httpClient, httpRetries, externalService, e.e.Config)
	}

	return nil, errors.Errorf("unrecognized external service type '%s'", e.Type())
}

func (e *ExternalService) UpdateConfig(ctx context.Context, db Queryer, add map[string]string, remove map[string]bool) error {
	for key, value := range add {
		e.e.Config[key] = value
	}
	for key := range remove {
		delete(e.e.Config, key)
	}

	dbMap := make(map[string]interface{}, len(e.e.Config))
	for key, value := range e.e.Config {
		dbMap[key] = value
	}

	return Exec(ctx, "update external service config", db, `UPDATE externals_externalservice SET config = $2 WHERE id = $1`, e.e.ID, null.NewMap(dbMap))
}

type ExternalServiceService interface {
	flows.ExternalServiceService
}

type ExternalServiceServiceFunc func(*runtime.Config, *http.Client, *httpx.RetryConfig, *flows.ExternalService, map[string]string) (ExternalServiceService, error)

var externalServiceServices = map[string]ExternalServiceServiceFunc{}

func RegisterExternalServiceService(name string, initFunc ExternalServiceServiceFunc) {
	externalServiceServices[name] = initFunc
}

const selectExternalServiceByUUIDSQL = `
SELECT ROW_TO_JSON(r) FROM (SELECT
	e.id as id,
	e.uuid as uuid,
	e.org_id as org_id,
	e.name as name,
	e.external_service_type as external_service_type,
	e.config as config
FROM
	externals_externalservice e
WHERE
	e.uuid = $1 AND
	e.is_active = TRUE
) r;
`

func LookupExternalServiceByUUID(ctx context.Context, db Queryer, uuid assets.ExternalServiceUUID) (*ExternalService, error) {
	rows, err := db.QueryxContext(ctx, selectExternalServiceByUUIDSQL, string(uuid))
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrapf(err, "error querying for external service for uuid: %s", string(uuid))
	}
	defer rows.Close()

	if err == sql.ErrNoRows || !rows.Next() {
		return nil, nil
	}

	externalService := &ExternalService{}
	err = dbutil.ReadJSONRow(rows, &externalService.e)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling external service")
	}

	return externalService, nil
}

const selectOrgExternalServicesSQL = `
SELECT ROW_TO_JSON(r) FROM (SELECT
	e.id as id,
	e.uuid as uuid,
	e.org_id as org_id,
	e.name as name,
	e.external_service_type as external_service_type,
	e.config as config
FROM
	externals_externalservice e
WHERE
	e.org_id = $1 AND
	e.is_active = TRUE
ORDER BY
	e.created_on ASC
) r;
`

func loadExternalServices(ctx context.Context, db sqlx.Queryer, orgID OrgID) ([]assets.ExternalService, error) {
	start := time.Now()

	rows, err := db.Queryx(selectOrgExternalServicesSQL, orgID)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrapf(err, "error querying external service for org: %d", orgID)
	}
	defer rows.Close()

	externalServices := make([]assets.ExternalService, 0)
	for rows.Next() {
		externalService := &ExternalService{}
		err := dbutil.ReadJSONRow(rows, &externalService.e)
		if err != nil {
			return nil, errors.Wrapf(err, "error unmarshalling external service")
		}
		externalServices = append(externalServices, externalService)
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("org_id", orgID).WithField("count", len(externalServices)).Debug("loaded external services")

	return externalServices, nil
}

type PromptID null.Int
type PromptUUID uuids.UUID

type Prompt struct {
	p struct {
		ID               PromptID          `db:"id"`
		UUID             PromptUUID        `db:"uuid"`
		Text             string            `db:"text"`
		ChatGPTServiceID ExternalServiceID `db:"chat_gpt_service_id"`
	}
}

func (p *Prompt) ID() PromptID                         { return p.p.ID }
func (p *Prompt) UUID() PromptUUID                     { return p.p.UUID }
func (p *Prompt) Text() string                         { return p.p.Text }
func (p *Prompt) ExternalServiceID() ExternalServiceID { return p.p.ChatGPTServiceID }

const selectPromptsByExternalServiceIDSQL = `
SELECT 
	p.id as id,
	p.uuid as uuid,
	p."text" as "text",
	p.chat_gpt_service_id as chat_gpt_service_id
FROM 
	public.externals_prompt p
WHERE
  p.chat_gpt_service_id = $1
`

func SelectPromptsByExternalServiceID(ctx context.Context, db Queryer, externalServiceID ExternalServiceID) ([]*Prompt, error) {
	rows, err := db.QueryxContext(ctx, selectPromptsByExternalServiceIDSQL, externalServiceID)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrap(err, "error loading prompts")
	}
	defer rows.Close()

	prompts := make([]*Prompt, 0, 2)
	for rows.Next() {
		prompt := &Prompt{}
		err = rows.StructScan(&prompt.p)
		if err != nil {
			return nil, errors.Wrapf(err, "error unmarshalling prompt")
		}
		prompts = append(prompts, prompt)
	}

	return prompts, nil
}
