package generic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOpenTemplate(t *testing.T) {
	_, err := parseOpenTemplate(`{"id":"{{.ticket_id}}"}`)
	require.NoError(t, err)

	_, err = parseOpenTemplate(`{{.ticket_id`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid open_template")
}

func TestParseForwardTemplate(t *testing.T) {
	_, err := parseForwardTemplate(`{"text":"{{.text}}"}`)
	require.NoError(t, err)

	_, err = parseForwardTemplate(`{{.text`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid forward_template")
}

func TestParseForwardResponseTemplate(t *testing.T) {
	_, err := parseForwardResponseTemplate(`{"message_external_id":"{{.id}}"}`)
	require.NoError(t, err)

	_, err = parseForwardResponseTemplate(`{{.id`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid forward_response_template")
}

func TestParseCloseTemplate(t *testing.T) {
	_, err := parseCloseTemplate(`{"id":"{{.external_id}}"}`)
	require.NoError(t, err)

	_, err = parseCloseTemplate(`{{.external_id`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid close_template")
}

func TestParseCloseResponseTemplate(t *testing.T) {
	_, err := parseCloseResponseTemplate(`{"status":"{{.state}}"}`)
	require.NoError(t, err)

	_, err = parseCloseResponseTemplate(`{{.state`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid close_response_template")
}

func TestParseMessagesTemplate(t *testing.T) {
	_, err := parseMessagesTemplate(`{"external_id":"{{.ticket}}","text":"{{.content}}"}`)
	require.NoError(t, err)

	_, err = parseMessagesTemplate(`{{.ticket`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid messages_template")
}

func TestMapAgentMessagePayload(t *testing.T) {
	tmpl, err := parseMessagesTemplate(`{"external_id":"{{.ticket}}","direction":"outgoing","text":"{{.content}}","sent_at":"2026-05-20T14:35:00Z"}`)
	require.NoError(t, err)

	payload, err := mapAgentMessagePayload(tmpl, []byte(`{"ticket":"EXT-1","content":"hello"}`))
	require.NoError(t, err)
	assert.Equal(t, "EXT-1", payload.ExternalID)
	assert.Equal(t, "outgoing", payload.Direction)
	assert.Equal(t, "hello", payload.Text)
}

func TestRenderMessagesResponseTemplate(t *testing.T) {
	tmpl, err := parseMessagesResponseTemplate(`{"ok":true,"id":"{{.message_uuid}}"}`)
	require.NoError(t, err)

	out, err := renderMessagesResponseTemplate(tmpl, map[string]interface{}{
		"status":       "sent",
		"message_uuid": "msg-1",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true,"id":"msg-1"}`, string(out))
}

func TestParseTicketsCloseTemplate(t *testing.T) {
	_, err := parseTicketsCloseTemplate(`{"external_id":"{{.ticket}}"}`)
	require.NoError(t, err)

	_, err = parseTicketsCloseTemplate(`{{.ticket`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tickets_close_template")
}

func TestMapCloseTicketPayload(t *testing.T) {
	tmpl, err := parseTicketsCloseTemplate(`{"external_id":"{{.ticket}}","reason":"{{.reason}}","closed_at":"{{.closed_at}}"}`)
	require.NoError(t, err)

	payload, err := mapCloseTicketPayload(tmpl, []byte(`{"ticket":"EXT-1","reason":"resolved","closed_at":"2026-05-20T14:50:00Z"}`))
	require.NoError(t, err)
	assert.Equal(t, "EXT-1", payload.ExternalID)
	assert.Equal(t, "resolved", payload.Reason)
}

func TestRenderTicketsCloseResponseTemplate(t *testing.T) {
	tmpl, err := parseTicketsCloseResponseTemplate(`{"ok":true,"ticket":"{{.ticket_uuid}}"}`)
	require.NoError(t, err)

	out, err := renderTicketsCloseResponseTemplate(tmpl, map[string]interface{}{
		"status":      "closed",
		"ticket_uuid": "ticket-1",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true,"ticket":"ticket-1"}`, string(out))
}

func TestMapCloseResponse(t *testing.T) {
	tmpl, err := parseCloseResponseTemplate(`{"status":"{{.result.state}}"}`)
	require.NoError(t, err)

	resp, err := mapCloseResponse(tmpl, []byte(`{"result":{"state":"closed"}}`))
	require.NoError(t, err)
	assert.Equal(t, "closed", resp.Status)
}

func TestRenderCloseTemplate(t *testing.T) {
	tmpl, err := parseCloseTemplate(`{"id":"{{.external_id}}","by":{{json .closed_by}},"at":"{{.closed_at}}"}`)
	require.NoError(t, err)

	req := &CloseRequest{
		TicketID:   "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
		ExternalID: "EXT-123",
		ClosedBy:   ActorRef{Type: "platform", ID: "system"},
		ClosedAt:   time.Date(2026, 5, 20, 14, 50, 0, 0, time.UTC),
	}

	out, err := renderCloseTemplate(tmpl, req)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"EXT-123",
		"by":{"type":"platform","id":"system"},
		"at":"2026-05-20T14:50:00Z"
	}`, string(out))
}

func TestMapForwardResponse(t *testing.T) {
	tmpl, err := parseForwardResponseTemplate(`{"message_external_id":"{{.result.id}}","status":"{{.result.state}}"}`)
	require.NoError(t, err)

	resp, err := mapForwardResponse(tmpl, []byte(`{"result":{"id":"MSG-9","state":"received"}}`))
	require.NoError(t, err)
	assert.Equal(t, "MSG-9", resp.MessageExternalID)
	assert.Equal(t, "received", resp.Status)
}

func TestRenderForwardTemplate(t *testing.T) {
	tmpl, err := parseForwardTemplate(`{"ticket":"{{.external_id}}","from":{{json .sender}},"body":"{{.text}}"}`)
	require.NoError(t, err)

	req := &MessageRequest{
		TicketID:   "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
		ExternalID: "EXT-123",
		Direction:  "incoming",
		Sender:     Sender{Type: "contact", ID: "c-1", Name: "João"},
		Text:       "Hello",
		SentAt:     time.Date(2026, 5, 20, 14, 32, 0, 0, time.UTC),
	}

	out, err := renderForwardTemplate(tmpl, req)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"ticket":"EXT-123",
		"from":{"type":"contact","id":"c-1","name":"João"},
		"body":"Hello"
	}`, string(out))
}

func TestParseOpenResponseTemplate(t *testing.T) {
	_, err := parseOpenResponseTemplate(`{"external_id":"{{.id}}"}`)
	require.NoError(t, err)

	_, err = parseOpenResponseTemplate(`{{.id`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid open_response_template")
}

func TestRenderOpenTemplate(t *testing.T) {
	tmpl, err := parseOpenTemplate(`{"id":"{{.ticket_id}}","customer":{{json .contact}},"subject":"{{.body}}"}`)
	require.NoError(t, err)

	req := &OpenRequest{
		TicketID: "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
		Contact: Contact{
			UUID: "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
			Name: "João Silva",
			URN:  "whatsapp:+5511999999999",
		},
		Body:     "Need help",
		OpenedAt: time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC),
	}

	out, err := renderOpenTemplate(tmpl, req)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
		"customer":{"uuid":"7ad9d98e-321f-4c61-9a50-79b1c7d7f621","name":"João Silva","urn":"whatsapp:+5511999999999"},
		"subject":"Need help"
	}`, string(out))
}

func TestMapOpenResponse(t *testing.T) {
	tmpl, err := parseOpenResponseTemplate(`{"external_id":"{{.data.id}}","status":"{{.data.state}}","created_at":"{{.data.created}}"}`)
	require.NoError(t, err)

	resp, err := mapOpenResponse(tmpl, []byte(`{
		"data": {"id":"EXT-999","state":"open","created":"2026-05-20T14:30:03Z"}
	}`))
	require.NoError(t, err)
	assert.Equal(t, "EXT-999", resp.ExternalID)
	assert.Equal(t, "open", resp.Status)
	assert.Equal(t, time.Date(2026, 5, 20, 14, 30, 3, 0, time.UTC), resp.CreatedAt.UTC())
}

func TestMapOpenResponseInvalidJSON(t *testing.T) {
	tmpl, err := parseOpenResponseTemplate(`not-json {{.id}}`)
	require.NoError(t, err)

	_, err = mapOpenResponse(tmpl, []byte(`{"id":"EXT-1"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestRenderOpenTemplateInvalidJSON(t *testing.T) {
	tmpl, err := parseOpenTemplate(`not-json {{.ticket_id}}`)
	require.NoError(t, err)

	_, err = renderOpenTemplate(tmpl, &OpenRequest{
		TicketID: "abc",
		Contact:  Contact{UUID: "u", URN: "tel:+1"},
		OpenedAt: time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestRenderOpenTemplateEmpty(t *testing.T) {
	tmpl, err := parseOpenTemplate(`{{if false}}{"x":1}{{end}}`)
	require.NoError(t, err)

	_, err = renderOpenTemplate(tmpl, &OpenRequest{
		TicketID: "abc",
		Contact:  Contact{UUID: "u", URN: "tel:+1"},
		OpenedAt: time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty body")
}
