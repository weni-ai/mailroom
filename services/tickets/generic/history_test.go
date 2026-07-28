package generic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHistoryMode(t *testing.T) {
	assert.Equal(t, historyModeBatch, parseHistoryMode(""))
	assert.Equal(t, historyModeBatch, parseHistoryMode("batch"))
	assert.Equal(t, historyModeOneByOne, parseHistoryMode("one_by_one"))
}

func TestParseHistoryBatchSize(t *testing.T) {
	assert.Equal(t, 50, parseHistoryBatchSize(""))
	assert.Equal(t, 10, parseHistoryBatchSize("10"))
	assert.Equal(t, 50, parseHistoryBatchSize("0"))
	assert.Equal(t, 50, parseHistoryBatchSize("bad"))
}

func TestChunkHistoryMessages(t *testing.T) {
	msgs := []HistoryMessage{
		{MessageID: "1"}, {MessageID: "2"}, {MessageID: "3"},
	}
	batches := chunkHistoryMessages(msgs, 2)
	require.Len(t, batches, 2)
	assert.Len(t, batches[0], 2)
	assert.Len(t, batches[1], 1)
}

func TestParseHistoryAfter(t *testing.T) {
	parsed, err := parseHistoryAfter("2026-05-20 14:20:00")
	require.NoError(t, err)
	assert.Equal(t, 2026, parsed.Year())

	_, err = parseHistoryAfter("not-a-date")
	require.Error(t, err)
}

func TestHistoryOneByOneTemplateContextFrom(t *testing.T) {
	sentAt := time.Date(2026, 5, 20, 14, 20, 0, 0, time.UTC)
	ctx := historyOneByOneTemplateContextFrom(&HistoryRequest{
		TicketID:   "ticket-1",
		ExternalID: "EXT-1",
		Contact:    Contact{UUID: "c-1", URN: "whatsapp:+551199"},
	}, HistoryMessage{
		MessageID: "m-1",
		Direction: "incoming",
		Sender:    Sender{Type: "contact", ID: "c-1"},
		Text:      "hello",
		SentAt:    sentAt,
	})
	assert.Equal(t, "ticket-1", ctx.TicketID)
	assert.Equal(t, "m-1", ctx.MessageID)
	assert.Equal(t, "hello", ctx.Text)
}
