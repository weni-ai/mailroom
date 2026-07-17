package generic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTicketer inserts a generic-type ticketer row directly via SQL. We
// avoid the testdata helpers because the codebase doesn't ship a
// testdata.Generic constant and the dump shared across tests doesn't include
// a generic ticketer. The row is removed by t.Cleanup so tests stay
// isolated.
//
// We first realign the id sequence with MAX(id) because the shared test DB
// dump can have its sequence trail behind the existing rows, which would
// otherwise make the INSERT collide on the primary key.
func seedTicketer(t *testing.T, db *sqlx.DB, secret string, skipWebhookHMAC bool) assets.TicketerUUID {
	t.Helper()
	return seedTicketerWithConfig(t, db, secret, skipWebhookHMAC, nil)
}

func seedTicketerWithConfig(t *testing.T, db *sqlx.DB, secret string, skipWebhookHMAC bool, extra map[string]string) assets.TicketerUUID {
	t.Helper()
	u := assets.TicketerUUID(uuids.New())

	db.MustExec(`SELECT setval(
		'tickets_ticketer_id_seq',
		GREATEST(COALESCE((SELECT MAX(id) FROM tickets_ticketer), 1), 1)
	)`)

	cfg := map[string]string{
		"base_url":  "https://partner.example.com",
		"api_token": "x",
	}
	if secret != "" {
		cfg["webhook_secret"] = secret
	}
	if skipWebhookHMAC {
		cfg["skip_webhook_hmac"] = "true"
	}
	for k, v := range extra {
		cfg[k] = v
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)

	db.MustExec(
		`INSERT INTO tickets_ticketer(uuid, org_id, name, ticketer_type, config, is_active, created_on, modified_on, created_by_id, modified_by_id)
		 VALUES ($1, 1, 'Generic Test', 'generic', $2, TRUE, NOW(), NOW(), 1, 1)`,
		u, string(raw),
	)
	t.Cleanup(func() {
		db.MustExec(`DELETE FROM tickets_ticketer WHERE uuid = $1`, u)
	})
	return u
}

// makeReq builds an http.Request preloaded with the chi URL param so the
// handler sees a routed-style request.
func makeReq(method, body string, ticketerUUID assets.TicketerUUID, sig, ts string) *http.Request {
	r := httptest.NewRequest(method, "http://test/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if sig != "" {
		r.Header.Set(headerSignature, sig)
	}
	if ts != "" {
		r.Header.Set(headerTimestamp, ts)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ticketer", string(ticketerUUID))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	return r
}

func decodeResp(t *testing.T, resp interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	out := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestHandleAgentMessage_TicketerNotFound(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()

	body := `{"external_id":"any","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	bogus := assets.TicketerUUID("00000000-0000-0000-0000-000000000000")
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, bogus, "sha256=deadbeef", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticketer_not_found", m["error"])
}

func TestHandleAgentMessage_InvalidSignature(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	u := seedTicketer(t, db, "real-secret", false)

	body := `{"external_id":"EXT-1","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, "sha256=deadbeef", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_signature", m["error"])
}

func TestHandleAgentMessage_MissingSignature(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	u := seedTicketer(t, db, "real-secret", false)

	body := `{"external_id":"EXT-1","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, "", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_signature", m["error"])
}

func TestHandleAgentMessage_SkipHMACAcceptsUnsignedRequest(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	u := seedTicketer(t, db, "", true)

	body := `{"external_id":"DOES-NOT-EXIST","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, "", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleAgentMessage_ExpiredTimestamp(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	body := `{"external_id":"EXT-1","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	// 10 minutes in the past — outside the 5-minute window
	ts := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)

	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ts), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "expired_request", m["error"])
}

func TestHandleAgentMessage_InvalidJSON(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	body := `not json`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_payload", m["error"])
}

func TestHandleAgentMessage_EmptyMessage(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	// Valid JSON but no text and no attachments — rejected as empty
	body := `{"external_id":"EXT-1","sent_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "empty_message", m["error"])
}

func TestHandleAgentMessage_TicketNotFound(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	body := `{"external_id":"DOES-NOT-EXIST","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleAgentMessage_WithMessagesTemplate(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketerWithConfig(t, db, secret, false, map[string]string{
		"messages_template": `{"external_id":"{{.ticket}}","direction":"outgoing","text":"{{.content}}","sent_at":"2026-05-20T14:35:00Z"}`,
	})

	// Partner-shaped body: mapped via messages_template before lookup.
	body := `{"ticket":"DOES-NOT-EXIST","content":"hi from agent"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleAgentMessage_WithMessagesTemplateInvalidJSON(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketerWithConfig(t, db, secret, false, map[string]string{
		"messages_template": `not-json {{.ticket}}`,
	})

	body := `{"ticket":"EXT-1","content":"hi"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_payload", m["error"])
}

func TestHandleAgentMessage_WithInvalidMessagesTemplateConfig(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketerWithConfig(t, db, secret, false, map[string]string{
		"messages_template": `{"external_id":"{{.ticket"`,
	})

	body := `{"ticket":"EXT-1","content":"hi"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "misconfigured", m["error"])
}

func TestRenderAgentMessageResponse_WithTemplate(t *testing.T) {
	ticketer := models.BuildTicketer(
		models.TicketerID(1),
		assets.TicketerUUID("11111111-2222-3333-4444-555555555555"),
		1,
		"generic",
		"Generic",
		map[string]string{
			"messages_response_template": `{"ok":true,"id":"{{.message_uuid}}","ticket":"{{.ticket_uuid}}"}`,
		},
	)

	resp, status, err := renderAgentMessageResponse(ticketer, map[string]interface{}{
		"status":       "sent",
		"ticket_uuid":  "ticket-1",
		"message_uuid": "msg-1",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	raw, ok := resp.(json.RawMessage)
	require.True(t, ok)
	assert.JSONEq(t, `{"ok":true,"id":"msg-1","ticket":"ticket-1"}`, string(raw))
}

func TestHandleCloseTicket_WithTicketsCloseTemplate(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketerWithConfig(t, db, secret, false, map[string]string{
		"tickets_close_template": `{"external_id":"{{.ticket}}","closed_at":"2026-05-20T14:50:00Z"}`,
	})

	body := `{"ticket":"DOES-NOT-EXIST","reason":"resolved"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleCloseTicket(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleCloseTicket_WithTicketsCloseTemplateInvalidJSON(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketerWithConfig(t, db, secret, false, map[string]string{
		"tickets_close_template": `not-json {{.ticket}}`,
	})

	body := `{"ticket":"EXT-1"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleCloseTicket(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_payload", m["error"])
}

func TestRenderCloseTicketResponse_WithTemplate(t *testing.T) {
	ticketer := models.BuildTicketer(
		models.TicketerID(1),
		assets.TicketerUUID("11111111-2222-3333-4444-555555555555"),
		1,
		"generic",
		"Generic",
		map[string]string{
			"tickets_close_response_template": `{"ok":true,"ticket":"{{.ticket_uuid}}","state":"{{.status}}"}`,
		},
	)

	resp, status, err := renderCloseTicketResponse(ticketer, map[string]interface{}{
		"status":      "closed",
		"ticket_uuid": "ticket-1",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	raw, ok := resp.(json.RawMessage)
	require.True(t, ok)
	assert.JSONEq(t, `{"ok":true,"ticket":"ticket-1","state":"closed"}`, string(raw))
}

func TestHandleCloseTicket_TicketNotFound(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	body := `{"external_id":"DOES-NOT-EXIST","closed_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleCloseTicket(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleReopenTicket_TicketNotFound(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret, false)

	body := `{"external_id":"DOES-NOT-EXIST","reopened_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleReopenTicket(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}
