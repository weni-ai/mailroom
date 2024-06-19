package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
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

type CatalogID null.Int

func (i CatalogID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

func (i *CatalogID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

func (i CatalogID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

func (i *CatalogID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// MsgCatalog represents a product catalog from Whatsapp channels.
type MsgCatalog struct {
	c struct {
		ID                CatalogID             `json:"id"`
		UUID              assets.MsgCatalogUUID `json:"uuid"`
		FacebookCatalogID string                `json:"facebook_catalog_id"`
		Name              string                `json:"name"`
		CreatedOn         time.Time             `json:"created_on"`
		ModifiedOn        time.Time             `json:"modified_on"`
		IsActive          bool                  `json:"is_active"`
		ChannelID         ChannelID             `json:"channel_id"`
		OrgID             OrgID                 `json:"org_id"`
		ChannelUUID       assets.ChannelUUID    `json:"channel_uuid"`
		Type              string                `json:"type"`
	}
}

func (c *MsgCatalog) ID() CatalogID                   { return c.c.ID }
func (c *MsgCatalog) UUID() assets.MsgCatalogUUID     { return c.c.UUID }
func (c *MsgCatalog) FacebookCatalogID() string       { return c.c.FacebookCatalogID }
func (c *MsgCatalog) Name() string                    { return c.c.Name }
func (c *MsgCatalog) CreatedOn() time.Time            { return c.c.CreatedOn }
func (c *MsgCatalog) ModifiedOn() time.Time           { return c.c.ModifiedOn }
func (c *MsgCatalog) IsActive() bool                  { return c.c.IsActive }
func (c *MsgCatalog) ChannelID() ChannelID            { return c.c.ChannelID }
func (c *MsgCatalog) OrgID() OrgID                    { return c.c.OrgID }
func (c *MsgCatalog) Type() string                    { return c.c.Type }
func (c *MsgCatalog) ChannelUUID() assets.ChannelUUID { return c.c.ChannelUUID }

func init() {
	goflow.RegisterMsgCatalogServiceFactory(msgCatalogServiceFactory)
}

func msgCatalogServiceFactory(c *runtime.Config) engine.MsgCatalogServiceFactory {
	return func(session flows.Session, msgCatalog *flows.MsgCatalog) (flows.MsgCatalogService, error) {
		return msgCatalog.Asset().(*MsgCatalog).AsService(c, msgCatalog)
	}
}

func (e *MsgCatalog) AsService(cfg *runtime.Config, msgCatalog *flows.MsgCatalog) (MsgCatalogService, error) {
	httpClient, httpRetries, _ := goflow.HTTP(cfg)

	initFunc := msgCatalogServices["msg_catalog"]
	if initFunc != nil {
		return initFunc(cfg, httpClient, httpRetries, msgCatalog, nil)
	}

	return nil, errors.Errorf("unrecognized product catalog %s", e.Name())
}

type MsgCatalogServiceFunc func(*runtime.Config, *http.Client, *httpx.RetryConfig, *flows.MsgCatalog, map[string]string) (MsgCatalogService, error)

var msgCatalogServices = map[string]MsgCatalogServiceFunc{}

type MsgCatalogService interface {
	flows.MsgCatalogService
}

func RegisterMsgCatalogService(name string, initFunc MsgCatalogServiceFunc) {
	msgCatalogServices[name] = initFunc
}

const getActiveCatalogSQL = `
SELECT  ROW_TO_JSON(r) FROM (SELECT 
	c.id as id,
	c.uuid as uuid,
	c.facebook_catalog_id  as facebook_catalog_id,
	c.name as name,
	c.created_on as created_on,
	c.modified_on as modified_on,
	c.is_active as is_active,
	c.channel_id as channel_id,
	c.org_id as org_id
FROM 
	public.wpp_products_catalog c
WHERE
	channel_id = $1 AND is_active = true
) r;
`

// GetActiveCatalogFromChannel returns the active catalog from the given channel
func GetActiveCatalogFromChannel(ctx context.Context, db sqlx.DB, channelID ChannelID) (*MsgCatalog, error) {
	var catalog MsgCatalog

	rows, err := db.QueryxContext(ctx, getActiveCatalogSQL, channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error getting active catalog for channelID: %d", channelID)
	}
	defer rows.Close()

	for rows.Next() {
		err = dbutil.ReadJSONRow(rows, &catalog.c)
		if err != nil {
			return nil, err
		}
	}

	return &catalog, nil
}

func loadCatalog(ctx context.Context, db *sqlx.DB, orgID OrgID) ([]assets.MsgCatalog, error) {
	start := time.Now()

	rows, err := db.Queryx(selectOrgCatalogSQL, orgID)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Wrapf(err, "error querying catalog for org: %d", orgID)
	}
	defer rows.Close()

	catalog := make([]assets.MsgCatalog, 0)
	for rows.Next() {
		msgCatalog := &MsgCatalog{}
		err := dbutil.ReadJSONRow(rows, &msgCatalog.c)
		if err != nil {
			return nil, errors.Wrapf(err, "error unmarshalling catalog")
		}
		channelUUID, err := ChannelUUIDForChannelID(ctx, db, msgCatalog.ChannelID())
		if err != nil {
			return nil, err
		}

		if err == nil && channelUUID == assets.ChannelUUID("") {
			continue
		}

		msgCatalog.c.ChannelUUID = channelUUID
		msgCatalog.c.Type = "msg_catalog"
		catalog = append(catalog, msgCatalog)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error iterating through rows")
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("org_id", orgID).WithField("count", len(catalog)).Debug("loaded catalog")

	return catalog, nil
}

const selectOrgCatalogSQL = `
SELECT ROW_TO_JSON(r) FROM (SELECT
	c.id as id,
	c.uuid as uuid,
	c.facebook_catalog_id as facebook_catalog_id,
	c.name as name,
	c.created_on as created_on,
	c.org_id as org_id,
	c.modified_on as modified_on,
	c.is_active as is_active,
	c.channel_id as channel_id
FROM
    public.wpp_products_catalog c
WHERE
	c.org_id = $1 AND
	c.is_active = TRUE
ORDER BY
	c.created_on ASC
) r;
`

// ChannelForChannelID returns the channel for the passed in channel ID if any
func ChannelUUIDForChannelID(ctx context.Context, db *sqlx.DB, channelID ChannelID) (assets.ChannelUUID, error) {
	var channelUUID assets.ChannelUUID
	err := db.GetContext(ctx, &channelUUID, `SELECT uuid FROM channels_channel WHERE id = $1 AND is_active = TRUE`, channelID)
	if err != nil && err != sql.ErrNoRows {
		return assets.ChannelUUID(""), errors.Wrapf(err, "no channel found with id: %d", channelID)
	}

	if err == sql.ErrNoRows {
		return assets.ChannelUUID(""), nil
	}

	return channelUUID, nil
}
