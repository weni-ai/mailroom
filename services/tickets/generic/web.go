package generic

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/tickets"
	"github.com/nyaruka/mailroom/web"
	"github.com/pkg/errors"
)

// Webhook header names (see docs/generic-ticketer-service.md section 1.2).
const (
	headerSignature = "X-Webhook-Signature"
	headerTimestamp = "X-Webhook-Timestamp"
	signaturePrefix = "sha256="

	// replayWindow is the maximum age accepted on X-Webhook-Timestamp before
	// the request is rejected as a possible replay.
	replayWindow = 5 * time.Minute

	// maxWebhookBodyBytes caps the body size we read from a partner webhook.
	// Generous enough for messages with metadata; tightened from the global
	// MaxRequestBytes (32MB) since these payloads should be small.
	maxWebhookBodyBytes = int64(1 << 20) // 1MB
)

func init() {
	base := "/mr/tickets/types/" + typeGeneric + "/event_callback/{ticketer:[a-f0-9\\-]+}"

	web.RegisterJSONRoute(http.MethodPost, base+"/messages", web.WithHTTPLogs(handleAgentMessage))
	web.RegisterJSONRoute(http.MethodPost, base+"/tickets/close", web.WithHTTPLogs(handleCloseTicket))
	web.RegisterJSONRoute(http.MethodPost, base+"/tickets/reopen", web.WithHTTPLogs(handleReopenTicket))
}

// agentMessagePayload mirrors docs/generic-ticketer-service.md section 3.1.
type agentMessagePayload struct {
	ExternalID        string                 `json:"external_id"          validate:"required"`
	MessageExternalID string                 `json:"message_external_id"`
	Direction         string                 `json:"direction"`
	Sender            *Sender                `json:"sender"`
	Text              string                 `json:"text"`
	Attachments       []Attachment           `json:"attachments"`
	Metadata          map[string]interface{} `json:"metadata"`
	SentAt            time.Time              `json:"sent_at"`
}

// closeTicketPayload mirrors docs/generic-ticketer-service.md section 3.2.
type closeTicketPayload struct {
	ExternalID string                 `json:"external_id"        validate:"required"`
	ClosedBy   *ActorRef              `json:"closed_by"`
	Reason     string                 `json:"reason"`
	Metadata   map[string]interface{} `json:"metadata"`
	ClosedAt   time.Time              `json:"closed_at"`
}

// reopenTicketPayload mirrors docs/generic-ticketer-service.md section 3.3.
type reopenTicketPayload struct {
	ExternalID string                 `json:"external_id"          validate:"required"`
	ReopenedBy *ActorRef              `json:"reopened_by"`
	Metadata   map[string]interface{} `json:"metadata"`
	ReopenedAt time.Time              `json:"reopened_at"`
}

// errBody is the standard error envelope echoed back to the partner. It
// matches the format documented in section 9 of the spec so partners get
// consistent shapes whether the error originated on their side or ours.
func errBody(code, message string) map[string]interface{} {
	return map[string]interface{}{"error": code, "message": message}
}

// readWebhook reads the raw body, verifies the HMAC signature against the
// ticketer's webhook_secret, validates the timestamp, then returns the
// ticketer record and the raw body bytes for further JSON parsing. Any
// failure returns a status code + error envelope ready to be returned from a
// handler.
func readWebhook(ctx context.Context, rt *runtime.Runtime, r *http.Request) (*models.Ticketer, []byte, interface{}, int) {
	ticketerUUID := assets.TicketerUUID(chi.URLParam(r, "ticketer"))

	ticketer, _, err := tickets.FromTicketerUUID(ctx, rt, ticketerUUID, typeGeneric)
	if err != nil {
		return nil, nil, errBody("ticketer_not_found", "no such ticketer"), http.StatusNotFound
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes+1))
	if err != nil {
		return nil, nil, errBody("invalid_request", "unable to read request body"), http.StatusBadRequest
	}
	if int64(len(body)) > maxWebhookBodyBytes {
		return nil, nil, errBody("payload_too_large", "request body exceeds limit"), http.StatusRequestEntityTooLarge
	}

	if !skipWebhookHMACValue(ticketer.Config(configSkipWebhookHMAC)) {
		secret := ticketer.Config(configWebhookSecret)
		if secret == "" {
			return nil, nil, errBody("misconfigured", "ticketer has no webhook secret"), http.StatusInternalServerError
		}

		if !verifySignature(secret, body, r.Header.Get(headerSignature)) {
			return nil, nil, errBody("invalid_signature", "HMAC verification failed"), http.StatusUnauthorized
		}

		if ts := r.Header.Get(headerTimestamp); ts != "" {
			if !verifyTimestamp(ts, time.Now()) {
				return nil, nil, errBody("expired_request", "X-Webhook-Timestamp outside the accepted window"), http.StatusUnauthorized
			}
		}
	}

	return ticketer, body, nil, 0
}

// verifySignature checks the HMAC-SHA256 of body against the value in the
// X-Webhook-Signature header. The header may be given as `sha256=<hex>` or
// `<hex>`; both forms are accepted. Returns false when secret or signature is
// missing.
func verifySignature(secret string, body []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	provided := strings.TrimPrefix(signatureHeader, signaturePrefix)
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison guards against timing attacks.
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(provided)))
}

// verifyTimestamp accepts either RFC3339 timestamps (preferred per spec) or
// unix seconds expressed as a numeric string. Returns false when the value
// can't be parsed or falls outside replayWindow centered on `now`.
func verifyTimestamp(tsHeader string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, tsHeader)
	if err != nil {
		secs, parseErr := strconv.ParseInt(tsHeader, 10, 64)
		if parseErr != nil || secs == 0 {
			return false
		}
		t = time.Unix(secs, 0)
	}
	delta := now.Sub(t)
	if delta < 0 {
		delta = -delta
	}
	return delta <= replayWindow
}

// handleAgentMessage processes outgoing messages from the partner (an agent
// reply) and dispatches them to the contact via tickets.SendReply.
func handleAgentMessage(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketer, body, errResp, status := readWebhook(ctx, rt, r)
	if errResp != nil {
		return errResp, status, nil
	}

	payload, errResp, status := decodeAgentMessagePayload(ticketer, body)
	if errResp != nil {
		return errResp, status, nil
	}

	if strings.TrimSpace(payload.Text) == "" && len(payload.Attachments) == 0 {
		return errBody("empty_message", "text or attachments are required"), http.StatusBadRequest, nil
	}
	if strings.TrimSpace(payload.ExternalID) == "" {
		return errBody("invalid_payload", "external_id is required"), http.StatusBadRequest, nil
	}

	ticket, err := models.LookupTicketByExternalID(ctx, rt.DB, ticketer.ID(), payload.ExternalID)
	if err != nil || ticket == nil {
		return errBody("ticket_not_found", "no such ticket for the given external_id"), http.StatusNotFound, nil
	}

	files := make([]*tickets.File, 0, len(payload.Attachments))
	for _, att := range payload.Attachments {
		if att.URL == "" {
			continue
		}
		f, err := tickets.FetchFile(att.URL, nil)
		if err != nil {
			return errBody("attachment_fetch_failed", errors.Wrapf(err, "failed fetching %s", att.URL).Error()), http.StatusBadGateway, nil
		}
		files = append(files, f)
	}

	msg, err := tickets.SendReply(ctx, rt, ticket, payload.Text, files, nil)
	if err != nil {
		return errBody("send_failed", err.Error()), http.StatusInternalServerError, nil
	}

	resp := map[string]interface{}{
		"status":      "sent",
		"ticket_uuid": ticket.UUID(),
	}
	if msg != nil {
		resp["message_uuid"] = msg.UUID()
	}

	return renderAgentMessageResponse(ticketer, resp)
}

// decodeAgentMessagePayload unmarshals the webhook body into the standard
// agent message shape, optionally mapping through messages_template first.
// HMAC verification already ran on the raw body in readWebhook.
func decodeAgentMessagePayload(ticketer *models.Ticketer, body []byte) (*agentMessagePayload, interface{}, int) {
	if src := strings.TrimSpace(ticketer.Config(configMessagesTemplate)); src != "" {
		tmpl, err := parseMessagesTemplate(src)
		if err != nil {
			return nil, errBody("misconfigured", err.Error()), http.StatusInternalServerError
		}
		payload, err := mapAgentMessagePayload(tmpl, body)
		if err != nil {
			return nil, errBody("invalid_payload", err.Error()), http.StatusBadRequest
		}
		return payload, nil, 0
	}

	payload := &agentMessagePayload{}
	if err := utils.UnmarshalAndValidateWithLimit(io.NopCloser(bytes.NewReader(body)), payload, maxWebhookBodyBytes); err != nil {
		return nil, errBody("invalid_payload", err.Error()), http.StatusBadRequest
	}
	return payload, nil, 0
}

// renderAgentMessageResponse optionally maps the platform success response
// through messages_response_template before returning it to the partner.
func renderAgentMessageResponse(ticketer *models.Ticketer, resp map[string]interface{}) (interface{}, int, error) {
	if src := strings.TrimSpace(ticketer.Config(configMessagesResponseTemplate)); src != "" {
		tmpl, err := parseMessagesResponseTemplate(src)
		if err != nil {
			return errBody("misconfigured", err.Error()), http.StatusInternalServerError, nil
		}
		out, err := renderMessagesResponseTemplate(tmpl, resp)
		if err != nil {
			return errBody("misconfigured", err.Error()), http.StatusInternalServerError, nil
		}
		return json.RawMessage(out), http.StatusOK, nil
	}
	return resp, http.StatusOK, nil
}

// handleCloseTicket marks the ticket closed on the platform side after
// receiving a close notification from the partner.
func handleCloseTicket(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketer, body, errResp, status := readWebhook(ctx, rt, r)
	if errResp != nil {
		return errResp, status, nil
	}

	payload, errResp, status := decodeCloseTicketPayload(ticketer, body)
	if errResp != nil {
		return errResp, status, nil
	}
	if strings.TrimSpace(payload.ExternalID) == "" {
		return errBody("invalid_payload", "external_id is required"), http.StatusBadRequest, nil
	}

	ticket, err := models.LookupTicketByExternalID(ctx, rt.DB, ticketer.ID(), payload.ExternalID)
	if err != nil || ticket == nil {
		return errBody("ticket_not_found", "no such ticket for the given external_id"), http.StatusNotFound, nil
	}

	if ticket.Status() == models.TicketStatusClosed {
		// Idempotent: partner can safely retry.
		return renderCloseTicketResponse(ticketer, map[string]interface{}{
			"status":      "closed",
			"ticket_uuid": ticket.UUID(),
		})
	}

	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		return errBody("internal", err.Error()), http.StatusInternalServerError, nil
	}

	// externally=true so we don't notify the partner back (they're the source
	// of truth for this close event).
	if err := tickets.Close(ctx, rt, oa, ticket, true, l, string(body)); err != nil {
		return errBody("close_failed", err.Error()), http.StatusInternalServerError, nil
	}

	return renderCloseTicketResponse(ticketer, map[string]interface{}{
		"status":      "closed",
		"ticket_uuid": ticket.UUID(),
	})
}

// decodeCloseTicketPayload unmarshals the webhook body into the standard close
// ticket shape, optionally mapping through tickets_close_template first.
// HMAC verification already ran on the raw body in readWebhook.
func decodeCloseTicketPayload(ticketer *models.Ticketer, body []byte) (*closeTicketPayload, interface{}, int) {
	if src := strings.TrimSpace(ticketer.Config(configTicketsCloseTemplate)); src != "" {
		tmpl, err := parseTicketsCloseTemplate(src)
		if err != nil {
			return nil, errBody("misconfigured", err.Error()), http.StatusInternalServerError
		}
		payload, err := mapCloseTicketPayload(tmpl, body)
		if err != nil {
			return nil, errBody("invalid_payload", err.Error()), http.StatusBadRequest
		}
		return payload, nil, 0
	}

	payload := &closeTicketPayload{}
	if err := utils.UnmarshalAndValidateWithLimit(io.NopCloser(bytes.NewReader(body)), payload, maxWebhookBodyBytes); err != nil {
		return nil, errBody("invalid_payload", err.Error()), http.StatusBadRequest
	}
	return payload, nil, 0
}

// renderCloseTicketResponse optionally maps the platform success response
// through tickets_close_response_template before returning it to the partner.
func renderCloseTicketResponse(ticketer *models.Ticketer, resp map[string]interface{}) (interface{}, int, error) {
	if src := strings.TrimSpace(ticketer.Config(configTicketsCloseResponseTemplate)); src != "" {
		tmpl, err := parseTicketsCloseResponseTemplate(src)
		if err != nil {
			return errBody("misconfigured", err.Error()), http.StatusInternalServerError, nil
		}
		out, err := renderTicketsCloseResponseTemplate(tmpl, resp)
		if err != nil {
			return errBody("misconfigured", err.Error()), http.StatusInternalServerError, nil
		}
		return json.RawMessage(out), http.StatusOK, nil
	}
	return resp, http.StatusOK, nil
}

// handleReopenTicket reopens the ticket on the platform side after receiving
// a reopen notification from the partner.
func handleReopenTicket(ctx context.Context, rt *runtime.Runtime, r *http.Request, l *models.HTTPLogger) (interface{}, int, error) {
	ticketer, body, errResp, status := readWebhook(ctx, rt, r)
	if errResp != nil {
		return errResp, status, nil
	}

	payload := &reopenTicketPayload{}
	if err := utils.UnmarshalAndValidateWithLimit(io.NopCloser(bytes.NewReader(body)), payload, maxWebhookBodyBytes); err != nil {
		return errBody("invalid_payload", err.Error()), http.StatusBadRequest, nil
	}

	ticket, err := models.LookupTicketByExternalID(ctx, rt.DB, ticketer.ID(), payload.ExternalID)
	if err != nil || ticket == nil {
		return errBody("ticket_not_found", "no such ticket for the given external_id"), http.StatusNotFound, nil
	}

	if ticket.Status() == models.TicketStatusOpen {
		return map[string]interface{}{"status": "open", "ticket_uuid": ticket.UUID()}, http.StatusOK, nil
	}

	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		return errBody("internal", err.Error()), http.StatusInternalServerError, nil
	}

	if err := tickets.Reopen(ctx, rt, oa, ticket, true, l); err != nil {
		return errBody("reopen_failed", err.Error()), http.StatusInternalServerError, nil
	}

	return map[string]interface{}{"status": "open", "ticket_uuid": ticket.UUID()}, http.StatusOK, nil
}
