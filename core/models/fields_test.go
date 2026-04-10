package models_test

import (
	"testing"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFields(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshFields)
	require.NoError(t, err)

	expectedFields := []struct {
		field     testdata.Field
		key       string
		name      string
		valueType assets.FieldType
	}{
		{*testdata.GenderField, "gender", "Gender", assets.FieldTypeText},
		{*testdata.AgeField, "age", "Age", assets.FieldTypeNumber},
		{*testdata.CreatedOnField, "created_on", "Created On", assets.FieldTypeDatetime},
		{*testdata.LastSeenOnField, "last_seen_on", "Last Seen On", assets.FieldTypeDatetime},
	}
	for _, tc := range expectedFields {
		field := oa.FieldByUUID(tc.field.UUID)
		require.NotNil(t, field, "no such field: %s", tc.field.UUID)

		fieldByKey := oa.FieldByKey(tc.key)
		assert.Equal(t, field, fieldByKey)

		assert.Equal(t, tc.field.UUID, field.UUID(), "uuid mismatch for field %s", tc.field.ID)
		assert.Equal(t, tc.key, field.Key())
		assert.Equal(t, tc.name, field.Name())
		assert.Equal(t, tc.valueType, field.Type())
	}
}

func TestGetOrCreateContactField(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetData)

	orgID := testdata.Org1.ID

	t.Run("creates a new field", func(t *testing.T) {
		err := models.GetOrCreateContactField(ctx, db, orgID, "segment", "Segment")
		require.NoError(t, err)

		testsuite.AssertQuery(t, db,
			`SELECT count(*) FROM contacts_contactfield WHERE org_id = $1 AND key = 'segment' AND is_active = TRUE AND value_type = 'T' AND field_type = 'U'`,
			orgID,
		).Returns(1)

		oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, orgID, models.RefreshFields)
		require.NoError(t, err)

		field := oa.FieldByKey("segment")
		require.NotNil(t, field)
		assert.Equal(t, "segment", field.Key())
		assert.Equal(t, "Segment", field.Name())
		assert.Equal(t, assets.FieldTypeText, field.Type())
	})

	t.Run("is idempotent", func(t *testing.T) {
		err := models.GetOrCreateContactField(ctx, db, orgID, "segment", "Segment")
		require.NoError(t, err)

		err = models.GetOrCreateContactField(ctx, db, orgID, "segment", "Segment")
		require.NoError(t, err)

		testsuite.AssertQuery(t, db,
			`SELECT count(*) FROM contacts_contactfield WHERE org_id = $1 AND key = 'segment'`,
			orgID,
		).Returns(1)
	})

	t.Run("reactivates a soft-deleted field", func(t *testing.T) {
		db.MustExec(`UPDATE contacts_contactfield SET is_active = FALSE WHERE org_id = $1 AND key = 'segment'`, orgID)

		testsuite.AssertQuery(t, db,
			`SELECT count(*) FROM contacts_contactfield WHERE org_id = $1 AND key = 'segment' AND is_active = TRUE`,
			orgID,
		).Returns(0)

		err := models.GetOrCreateContactField(ctx, db, orgID, "segment", "Segment")
		require.NoError(t, err)

		testsuite.AssertQuery(t, db,
			`SELECT count(*) FROM contacts_contactfield WHERE org_id = $1 AND key = 'segment' AND is_active = TRUE`,
			orgID,
		).Returns(1)
	})

	t.Run("creates multiple different fields", func(t *testing.T) {
		err := models.GetOrCreateContactField(ctx, db, orgID, "orderform", "Orderform")
		require.NoError(t, err)

		testsuite.AssertQuery(t, db,
			`SELECT count(*) FROM contacts_contactfield WHERE org_id = $1 AND key = 'orderform' AND is_active = TRUE`,
			orgID,
		).Returns(1)

		oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, orgID, models.RefreshFields)
		require.NoError(t, err)

		assert.NotNil(t, oa.FieldByKey("segment"))
		assert.NotNil(t, oa.FieldByKey("orderform"))
	})
}
