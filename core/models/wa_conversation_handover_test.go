package models_test

import (
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupAndConsumeWAConversationHandover(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)

	channel := testdata.InsertChannel(db, testdata.Org1, "WA", "Handover Channel", []string{"whatsapp"}, "SR", map[string]interface{}{})
	contact := testdata.InsertContact(db, testdata.Org1, flows.ContactUUID(uuids.New()), "Handover Contact", envs.Language("eng"))
	urn := urns.URN("whatsapp:250700000077")
	testdata.InsertContactURN(db, testdata.Org1, contact, urn, 1000)
	contact.URN = urn

	db.MustExec(`DELETE FROM wa_conversation_handover WHERE channel_id = $1 AND contact_id = $2`, channel.ID, contact.ID)

	var handoverID int64
	err := db.Get(&handoverID, `
		INSERT INTO wa_conversation_handover(org_id, channel_id, contact_id, contact_urn, context_type, context_text, occurred_on, created_on)
		VALUES ($1, $2, $3, $4, 'summary', 'Customer asked about order #123', NOW(), NOW()) RETURNING id`,
		testdata.Org1.ID, channel.ID, contact.ID, contact.URN.String(),
	)
	require.NoError(t, err)

	pending, err := models.LookupPendingWAConversationHandover(ctx, db, channel.ID, contact.ID)
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.Equal(t, handoverID, pending.ID)
	assert.Equal(t, "Customer asked about order #123", pending.ContextText)

	consumed, err := models.ConsumePendingWAConversationHandover(ctx, db, pending.ID, flows.MsgID(999))
	require.NoError(t, err)
	assert.True(t, consumed)

	consumed, err = models.ConsumePendingWAConversationHandover(ctx, db, pending.ID, flows.MsgID(1000))
	require.NoError(t, err)
	assert.False(t, consumed)

	pending, err = models.LookupPendingWAConversationHandover(ctx, db, channel.ID, contact.ID)
	require.NoError(t, err)
	assert.Nil(t, pending)

	testsuite.AssertQuery(t, db, `SELECT consumed_msg_id FROM wa_conversation_handover WHERE id = $1`, handoverID).Columns(map[string]interface{}{"consumed_msg_id": int64(999)})
}
