package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/utils/dbutil"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

type WhatsAppFlowID null.Int

func (i WhatsAppFlowID) MarshalJSON() ([]byte, error) {
	return null.Int(i).MarshalJSON()
}

func (i *WhatsAppFlowID) UnmarshalJSON(b []byte) error {
	return null.UnmarshalInt(b, (*null.Int)(i))
}

func (i WhatsAppFlowID) Value() (driver.Value, error) {
	return null.Int(i).Value()
}

func (i *WhatsAppFlowID) Scan(value interface{}) error {
	return null.ScanInt(value, (*null.Int)(i))
}

// WhatsAppFlow representation for WhatsApp channels
type WhatsAppFlow struct {
	f struct {
		ID             CatalogID          `json:"id"`
		UUID           uuids.UUID         `json:"uuid"`
		FacebookFlowID string             `json:"facebook_flow_id"`
		Category       []string           `json:"category"`
		Status         string             `json:"status"`
		Name           string             `json:"name"`
		CreatedOn      time.Time          `json:"created_on"`
		ModifiedOn     time.Time          `json:"modified_on"`
		IsActive       bool               `json:"is_active"`
		ChannelID      ChannelID          `json:"channel_id"`
		OrgID          OrgID              `json:"org_id"`
		ChannelUUID    assets.ChannelUUID `json:"channel_uuid"`
	}
}

func (f *WhatsAppFlow) ID() CatalogID                   { return f.f.ID }
func (f *WhatsAppFlow) UUID() uuids.UUID                { return f.f.UUID }
func (f *WhatsAppFlow) FacebookCatalogID() string       { return f.f.FacebookFlowID }
func (f *WhatsAppFlow) Category() []string              { return f.f.Category }
func (f *WhatsAppFlow) Status() string                  { return f.f.Status }
func (f *WhatsAppFlow) Name() string                    { return f.f.Name }
func (f *WhatsAppFlow) CreatedOn() time.Time            { return f.f.CreatedOn }
func (f *WhatsAppFlow) ModifiedOn() time.Time           { return f.f.ModifiedOn }
func (f *WhatsAppFlow) IsActive() bool                  { return f.f.IsActive }
func (f *WhatsAppFlow) ChannelID() ChannelID            { return f.f.ChannelID }
func (f *WhatsAppFlow) OrgID() OrgID                    { return f.f.OrgID }
func (f *WhatsAppFlow) ChannelUUID() assets.ChannelUUID { return f.f.ChannelUUID }

const getActiveWhatsAppFlowSQL = `
SELECT  ROW_TO_JSON(r) FROM (SELECT
	f.id as id,
	f.uuid as uuid,
	f.facebook_flow_id  as facebook_flow_id,
	f.category as category,
	f.status as status,
	f.name as name,
	f.created_on as created_on,
	f.modified_on as modified_on,
	f.is_active as is_active,
	f.channel_id as channel_id,
	f.org_id as org_id
FROM
	public.wpp_flows_whatsappflow f
WHERE
	channel_id = $1 AND is_active = true AND facebook_flow_id = $2
) r;
`

// GetActiveCatalogFromChannel returns the active catalog from the given channel
func GetActiveWhatsAppFlowFromFacebookIDAndChannel(ctx context.Context, db Queryer, channelID ChannelID, facebookFlowID string) (*WhatsAppFlow, error) {
	var whatsAppFlow WhatsAppFlow

	rows, err := db.QueryxContext(ctx, getActiveWhatsAppFlowSQL, channelID, facebookFlowID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error getting active flow with id: %s, for channelID: %d", facebookFlowID, channelID)
	}
	defer rows.Close()

	for rows.Next() {
		err = dbutil.ReadJSONRow(rows, &whatsAppFlow.f)
		if err != nil {
			return nil, err
		}
	}

	return &whatsAppFlow, nil
}
