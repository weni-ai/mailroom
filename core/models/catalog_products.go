package models

import (
	"database/sql/driver"
	"net/http"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/mailroom/core/goflow"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
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

// CatalogProduct represents a product catalog from Whatsapp channels.
type CatalogProduct struct {
	c struct {
		ID                CatalogID  `db:"id"`
		UUID              uuids.UUID `db:"uuid"`
		FacebookCatalogID string     `db:"facebook_catalog_id"`
		Name              string     `db:"name"`
		CreatedOn         time.Time  `db:"created_on"`
		ModifiedOn        time.Time  `db:"modified_on"`
		IsActive          bool       `db:"is_active"`
		ChannelID         ChannelID  `db:"channel_id"`
		OrgID             OrgID      `db:"org_id"`
	}
}

func (c *CatalogProduct) ID() CatalogID             { return c.c.ID }
func (c *CatalogProduct) UUID() uuids.UUID          { return c.c.UUID }
func (c *CatalogProduct) FacebookCatalogID() string { return c.c.FacebookCatalogID }
func (c *CatalogProduct) Name() string              { return c.c.Name }
func (c *CatalogProduct) CreatedOn() time.Time      { return c.c.CreatedOn }
func (c *CatalogProduct) ModifiedOn() time.Time     { return c.c.ModifiedOn }
func (c *CatalogProduct) IsActive() bool            { return c.c.IsActive }
func (c *CatalogProduct) ChannelID() ChannelID      { return c.c.ChannelID }
func (c *CatalogProduct) OrgID() OrgID              { return c.c.OrgID }

type MsgCatalog struct {
	e struct {
		ID          CatalogID         `json:"id,omitempty"`
		ChannelUUID uuids.UUID        `json:"uuid,omitempty"`
		OrgID       OrgID             `json:"org_id,omitempty"`
		Name        string            `json:"name,omitempty"`
		Config      map[string]string `json:"config,omitempty"`
		Type        string            `json:"type,omitempty"`
	}
}

func (c *MsgCatalog) ChannelUUID() uuids.UUID { return c.e.ChannelUUID }
func (c *MsgCatalog) Name() string            { return c.e.Name }
func (c *MsgCatalog) Type() string            { return c.e.Type }

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
		return initFunc(cfg, httpClient, httpRetries, msgCatalog, e.e.Config)
	}

	return nil, errors.Errorf("unrecognized product catalog '%s'", e.e.Name)
}

type MsgCatalogServiceFunc func(*runtime.Config, *http.Client, *httpx.RetryConfig, *flows.MsgCatalog, map[string]string) (MsgCatalogService, error)

var msgCatalogServices = map[string]MsgCatalogServiceFunc{}

type MsgCatalogService interface {
	flows.MsgCatalogService
}

func RegisterMsgCatalogService(name string, initFunc MsgCatalogServiceFunc) {
	msgCatalogServices[name] = initFunc
}
