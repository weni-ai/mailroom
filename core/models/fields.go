package models

import (
	"context"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/utils/dbutil"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FieldID is our type for the database field ID
type FieldID int

// Field is our mailroom type for contact field types
type Field struct {
	f struct {
		ID        FieldID          `json:"id"`
		UUID      assets.FieldUUID `json:"uuid"`
		Key       string           `json:"key"`
		Name      string           `json:"name"`
		FieldType assets.FieldType `json:"field_type"`
		System    bool             `json:"is_system"`
	}
}

// ID returns the ID of this field
func (f *Field) ID() FieldID { return f.f.ID }

// UUID returns the UUID of this field
func (f *Field) UUID() assets.FieldUUID { return f.f.UUID }

// Key returns the key for this field
func (f *Field) Key() string { return f.f.Key }

// Name returns the name for this field
func (f *Field) Name() string { return f.f.Name }

// Type returns the value type for this field
func (f *Field) Type() assets.FieldType { return f.f.FieldType }

// System returns whether this is a system field
func (f *Field) System() bool { return f.f.System }

// loadFields loads the assets for the passed in db
func loadFields(ctx context.Context, db sqlx.Queryer, orgID OrgID) ([]assets.Field, []assets.Field, error) {
	start := time.Now()

	rows, err := db.Queryx(selectFieldsSQL, orgID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error querying fields for org: %d", orgID)
	}
	defer rows.Close()

	userFields := make([]assets.Field, 0, 10)
	systemFields := make([]assets.Field, 0, 10)

	for rows.Next() {
		field := &Field{}
		err = dbutil.ReadJSONRow(rows, &field.f)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error reading field")
		}

		if field.System() {
			systemFields = append(systemFields, field)
		} else {
			userFields = append(userFields, field)
		}
	}

	logrus.WithField("elapsed", time.Since(start)).WithField("org_id", orgID).WithField("count", len(userFields)).Debug("loaded contact fields")

	return userFields, systemFields, nil
}

const selectFieldsSQL = `
SELECT ROW_TO_JSON(f) FROM (SELECT
	id,
	uuid,
	key,
	label as name,
	(SELECT CASE value_type
		WHEN 'T' THEN 'text' 
		WHEN 'N' THEN 'number'
		WHEN 'D' THEN 'datetime'
		WHEN 'S' THEN 'state'
		WHEN 'I' THEN 'district'
		WHEN 'W' THEN 'ward'
	END) as field_type,
	field_type = 'S' as is_system
FROM
	contacts_contactfield 
WHERE 
	org_id = $1 AND 
	is_active = TRUE
ORDER BY
	key ASC
) f;
`

// GetOrCreateContactField ensures a contact field exists for the given org, creating it as a text
// field if necessary. If the field was previously soft-deleted, it is reactivated.
func GetOrCreateContactField(ctx context.Context, db Queryer, orgID OrgID, key string, label string) error {
	var exists bool
	err := db.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM contacts_contactfield WHERE org_id = $1 AND key = $2)`,
		orgID, key)
	if err != nil {
		return errors.Wrapf(err, "error checking contact field '%s' for org %d", key, orgID)
	}

	if exists {
		_, err = db.ExecContext(ctx,
			`UPDATE contacts_contactfield SET is_active = TRUE, modified_on = NOW() WHERE org_id = $1 AND key = $2`,
			orgID, key)
	} else {
		_, err = db.ExecContext(ctx, insertContactFieldSQL, uuids.New(), key, label, orgID)
	}
	if err != nil {
		return errors.Wrapf(err, "error creating contact field '%s' for org %d", key, orgID)
	}
	return nil
}

const insertContactFieldSQL = `
INSERT INTO contacts_contactfield (uuid, key, label, value_type, field_type, show_in_table, priority, is_active, org_id, created_on, modified_on, created_by_id, modified_by_id)
VALUES ($1, $2, $3, 'T', 'U', FALSE, 0, TRUE, $4, NOW(), NOW(), 1, 1)
`
