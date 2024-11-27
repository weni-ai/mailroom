package models

import (
	"database/sql/driver"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

type OrgContextID null.Int

func (i OrgContextID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

func (i *OrgContextID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

func (i OrgContextID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

func (i *OrgContextID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

func init() {
	goflow.RegisterOrgContextServiceFactory(orgContextServiceFactory)
}

func orgContextServiceFactory(c *runtime.Config) engine.OrgContextServiceFactory {
	return func(session flows.Session, orgContext *flows.OrgContext) (flows.OrgContextService, error) {
		return orgContext.Asset().(*OrgContext).AsService(c, orgContext)
	}
}

type OrgContext struct {
	c struct {
		OrgContext      string             `json:"context"`
		ChannelUUID     assets.ChannelUUID `json:"channel_uuid"`
		ProjectUUID     uuids.UUID         `json:"project_uuid"`
		HasVtexAds      bool               `json:"vtex_ads"`
		HideUnavailable bool               `json:"hide_unavailable"`
	}
}

func (c *OrgContext) Context() string                 { return c.c.OrgContext }
func (c *OrgContext) ChannelUUID() assets.ChannelUUID { return c.c.ChannelUUID }
func (c *OrgContext) ProjectUUID() uuids.UUID         { return c.c.ProjectUUID }
func (c *OrgContext) HasVtexAds() bool                { return c.c.HasVtexAds }
func (c *OrgContext) HideUnavailable() bool           { return c.c.HideUnavailable }

type OrgContextService interface {
	flows.OrgContextService
}

func (c *OrgContext) AsService(cfg *runtime.Config, context *flows.OrgContext) (OrgContextService, error) {
	httpClient, httpRetries, _ := goflow.HTTP(cfg)

	initFunc := orgContextServices["org_context"]
	if initFunc != nil {
		return initFunc(cfg, httpClient, httpRetries, context, nil)
	}

	return nil, errors.Errorf("unrecognized context %s", c.Context())
}

type OrgContextServiceFunc func(*runtime.Config, *http.Client, *httpx.RetryConfig, *flows.OrgContext, map[string]string) (OrgContextService, error)

var orgContextServices = map[string]OrgContextServiceFunc{}

func RegisterContextService(name string, initFunc OrgContextServiceFunc) {
	orgContextServices[name] = initFunc
}
