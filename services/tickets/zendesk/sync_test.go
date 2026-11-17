package zendesk

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkInt64s(t *testing.T) {
	assert.Nil(t, chunkInt64s(nil, 100))
	assert.Equal(t, [][]int64{{1, 2}, {3}}, chunkInt64s([]int64{1, 2, 3}, 2))
	assert.Equal(t, [][]int64{{1, 2, 3}}, chunkInt64s([]int64{1, 2, 3}, 100))
}

func TestClassifyOpenTickets(t *testing.T) {
	missing := models.NewTicket("11111111-1111-1111-1111-111111111111", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "", testdata.DefaultTopic.ID, "a", models.NilUserID, nil)
	open := models.NewTicket("22222222-2222-2222-2222-222222222222", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "10", testdata.DefaultTopic.ID, "b", models.NilUserID, nil)
	solved := models.NewTicket("33333333-3333-3333-3333-333333333333", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "11", testdata.DefaultTopic.ID, "c", models.NilUserID, nil)
	gone := models.NewTicket("44444444-4444-4444-4444-444444444444", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "12", testdata.DefaultTopic.ID, "d", models.NilUserID, nil)
	bad := models.NewTicket("55555555-5555-5555-5555-555555555555", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "abc", testdata.DefaultTopic.ID, "e", models.NilUserID, nil)

	report := newSyncReport(assets.TicketerUUID(testdata.Zendesk.UUID), false)
	classifyOpenTickets([]*models.Ticket{missing, open, solved, gone, bad}, map[int64]string{10: "open", 11: "solved"}, report)

	require.Len(t, report.MissingExternalID, 1)
	require.Len(t, report.InSync, 1)
	require.Len(t, report.WouldClose, 1)
	require.Len(t, report.NotFound, 2)
	assert.Empty(t, report.Closed)
	assert.NotNil(t, report.Closed)
	assert.Equal(t, 5, report.Counts.OpenInMailroom)
	assert.Equal(t, 1, report.Counts.InSync)
	assert.Equal(t, 1, report.Counts.WouldClose)
	assert.Equal(t, 0, report.Counts.Closed)
	assert.Equal(t, 1, report.Counts.MissingExternalID)
	assert.Equal(t, 2, report.Counts.NotFound)
	assert.Equal(t, "10", report.InSync[0].ExternalID)
	assert.Equal(t, "open", report.InSync[0].ZendeskStatus)
	assert.Equal(t, "11", report.WouldClose[0].ExternalID)
	assert.Equal(t, "solved", report.WouldClose[0].ZendeskStatus)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", report.MissingExternalID[0].UUID)
	assert.Empty(t, report.MissingExternalID[0].ExternalID)
	assert.Equal(t, "12", report.NotFound[0].ExternalID)
	assert.Equal(t, "abc", report.NotFound[1].ExternalID)
	assert.Empty(t, report.NotFound[0].ZendeskStatus)
	assert.Empty(t, report.NotFound[1].ZendeskStatus)
}

func TestSyncReportEmptySlicesMarshalAsArrays(t *testing.T) {
	report := newSyncReport(assets.TicketerUUID(testdata.Zendesk.UUID), true)
	raw, err := json.Marshal(report)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"in_sync", "would_close", "closed", "missing_external_id", "not_found"} {
		val, ok := decoded[key]
		require.True(t, ok, key)
		arr, ok := val.([]interface{})
		require.True(t, ok, key)
		assert.Empty(t, arr)
	}
	assert.Equal(t, true, decoded["sync"])
	assert.Equal(t, string(testdata.Zendesk.UUID), decoded["ticketer"])
}
