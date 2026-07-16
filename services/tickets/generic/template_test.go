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
