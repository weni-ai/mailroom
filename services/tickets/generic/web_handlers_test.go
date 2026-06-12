package generic

import (
	"context"
	"encoding/json"
	"fmt"
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
func seedTicketer(t *testing.T, db *sqlx.DB, secret string) assets.TicketerUUID {
	t.Helper()
	u := assets.TicketerUUID(uuids.New())

	db.MustExec(`SELECT setval(
		'tickets_ticketer_id_seq',
		GREATEST(COALESCE((SELECT MAX(id) FROM tickets_ticketer), 1), 1)
	)`)

	config := fmt.Sprintf(
		`{"base_url":"https://partner.example.com","api_token":"x","webhook_secret":%q}`,
		secret,
	)
	db.MustExec(
		`INSERT INTO tickets_ticketer(uuid, org_id, name, ticketer_type, config, is_active, created_on, modified_on, created_by_id, modified_by_id)
		 VALUES ($1, 1, 'Generic Test', 'generic', $2, TRUE, NOW(), NOW(), 1, 1)`,
		u, config,
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

	u := seedTicketer(t, db, "real-secret")

	body := `{"external_id":"EXT-1","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, "sha256=deadbeef", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_signature", m["error"])
}

func TestHandleAgentMessage_MissingSignature(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	u := seedTicketer(t, db, "real-secret")

	body := `{"external_id":"EXT-1","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, "", ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "invalid_signature", m["error"])
}

func TestHandleAgentMessage_ExpiredTimestamp(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret)

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
	u := seedTicketer(t, db, secret)

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
	u := seedTicketer(t, db, secret)

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
	u := seedTicketer(t, db, secret)

	body := `{"external_id":"DOES-NOT-EXIST","text":"hi","sent_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleAgentMessage(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}

func TestHandleCloseTicket_TicketNotFound(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	const secret = "real-secret"
	u := seedTicketer(t, db, secret)

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
	u := seedTicketer(t, db, secret)

	body := `{"external_id":"DOES-NOT-EXIST","reopened_at":"2026-05-20T14:30:00Z"}`
	sig := "sha256=" + computeSig(secret, body)
	resp, status, err := handleReopenTicket(ctx, rt, makeReq("POST", body, u, sig, ""), &models.HTTPLogger{})

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	m := decodeResp(t, resp)
	assert.Equal(t, "ticket_not_found", m["error"])
}
