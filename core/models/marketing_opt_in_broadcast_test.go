package models_test

import (
	"fmt"
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWppBroadcastMarketingOptOut(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetData)

	db.MustExec(`UPDATE contacts_contacturn SET identity = 'whatsapp:559899999999', path='559899999999', scheme='whatsapp' WHERE contact_id = $1`, testdata.Alexandria.ID)
	db.MustExec(`UPDATE contacts_contact SET language='eng' WHERE id = $1`, testdata.Alexandria.ID)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)

	templates, err := oa.Templates()
	require.NoError(t, err)
	require.True(t, len(templates) > 2)

	welcome := templates[2]
	db.MustExec(`UPDATE templates_template SET category = 'marketing' WHERE org_id = $1 AND uuid = $2`, testdata.Org1.ID, welcome.UUID())

	oa, err = models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTemplates)
	require.NoError(t, err)

	templateMsg := models.WppBroadcastMessage{
		Text: "hello @contact.name",
		Template: models.WppBroadcastTemplate{
			UUID:      welcome.UUID(),
			Name:      welcome.Name(),
			Variables: []string{"@contact.name"},
			Locale:    "eng",
		},
	}

	makeBroadcast := func() *models.WppBroadcast {
		return models.NewWppBroadcast(
			oa.OrgID(),
			models.NilBroadcastID,
			templateMsg,
			[]urns.URN{urns.URN("whatsapp:559899999999")},
			nil,
			nil,
			testdata.WhatsAppCloudChannel.ID,
			"batch",
		)
	}

	t.Run("contact without marketing_opt_in receives queued message", func(t *testing.T) {
		bcast := makeBroadcast()
		batch := bcast.CreateBatch([]models.ContactID{testdata.Alexandria.ID})

		msgs, err := models.CreateWppBroadcastMessages(ctx, rt, oa, batch)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Equal(t, models.MsgStatusQueued, msgs[0].Status())
	})

	t.Run("contact with marketing_opt_in false gets failed message", func(t *testing.T) {
		err := models.GetOrCreateContactField(ctx, db, testdata.Org1.ID, models.MarketingOptInFieldKey, "Marketing opt-in")
		require.NoError(t, err)

		oa, err = models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshFields)
		require.NoError(t, err)

		field := oa.FieldByKey(models.MarketingOptInFieldKey)
		require.NotNil(t, field)

		db.MustExec(
			fmt.Sprintf(`UPDATE contacts_contact SET fields = '{"%s": {"text": "false"}}' WHERE id = $1`, field.UUID()),
			testdata.Alexandria.ID,
		)
		models.FlushCache()

		bcast := makeBroadcast()
		batch := bcast.CreateBatch([]models.ContactID{testdata.Alexandria.ID})

		msgs, err := models.CreateWppBroadcastMessages(ctx, rt, oa, batch)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Equal(t, models.MsgStatusFailed, msgs[0].Status())
		assert.Equal(t, models.MsgFailedMarketingOptOut, msgs[0].FailedReason())
	})
}
