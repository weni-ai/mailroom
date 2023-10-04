package models

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

type CatalogID null.Int

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

const getActiveCatalogSQL = `
SELECT 
	id, uuid, facebook_catalog_id, name, created_on, modified_on, is_active, channel_id, org_id
FROM public.wpp_products_catalog
WHERE channel_id = $1 AND is_active = true
`

// GetActiveCatalogFromChannel returns the active catalog from the given channel
func GetActiveCatalogFromChannel(ctx context.Context, db sqlx.DB, channelID ChannelID) (*CatalogProduct, error) {
	var catalog CatalogProduct

	err := db.GetContext(ctx, &catalog.c, getActiveCatalogSQL, channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error getting active catalog for channelID: %d", channelID)
	}

	return &catalog, nil
}
