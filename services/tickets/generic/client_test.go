package generic_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/mailroom/services/tickets/generic"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBaseURL  = "https://partner.example.com"
	testAPIToken = "secret-token-123"
)

var (
	sampleTicketUUID  = "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71"
	sampleContactUUID = "7ad9d98e-321f-4c61-9a50-79b1c7d7f621"
	sampleExternalID  = "EXT-123456"
)

func resetGlobals(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		uuids.SetGenerator(uuids.DefaultGenerator)
		httpx.SetRequestor(httpx.DefaultRequestor)
	})
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
}

func newTestClient() *generic.Client {
	return generic.NewClient(http.DefaultClient, nil, testBaseURL, testAPIToken)
}

func openedAt() time.Time { return time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC) }
func sentAt() time.Time   { return time.Date(2026, 5, 20, 14, 32, 0, 0, time.UTC) }

func TestOpenTicket(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/v1/tickets": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-123456","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
			httpx.NewMockResponse(409, nil, `{"error":"ticket_already_open","message":"An open ticket already exists","details":{"external_id":"EXT-999"}}`),
		},
	}))

	client := newTestClient()

	req := &generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Topic:    &generic.Topic{UUID: "a1d2b8c3-9e4f-4a5b-8c6d-7e8f9a0b1c2d", Name: "Vendas"},
		Contact: generic.Contact{
			UUID: sampleContactUUID,
			Name: "João Silva",
			URN:  "whatsapp:+5511999999999",
		},
		Body:     "Cliente pediu atendimento humano.",
		OpenedAt: openedAt(),
	}
	idemKey := "open-" + sampleTicketUUID

	_, _, err := client.OpenTicket(req, idemKey)
	assert.EqualError(t, err, "unable to connect to server")

	resp, trace, err := client.OpenTicket(req, idemKey)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "EXT-123456", resp.ExternalID)
	assert.Equal(t, "open", resp.Status)
	assert.Equal(t, time.Date(2026, 5, 20, 14, 30, 3, 0, time.UTC), resp.CreatedAt.UTC())

	require.NotNil(t, trace)
	assert.Equal(t, "Bearer "+testAPIToken, trace.Request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", trace.Request.Header.Get("Content-Type"))
	assert.Equal(t, "1", trace.Request.Header.Get("X-API-Version"))
	assert.Equal(t, idemKey, trace.Request.Header.Get("Idempotency-Key"))
	assert.NotEmpty(t, trace.Request.Header.Get("X-Request-Id"))
	assert.Contains(t, string(trace.RequestTrace), `"ticket_id":"`+sampleTicketUUID+`"`)
	assert.Contains(t, string(trace.RequestTrace), `"opened_at":"2026-05-20T14:30:00Z"`)
	assert.Contains(t, string(trace.RequestTrace), `"topic":{"uuid":"a1d2b8c3-9e4f-4a5b-8c6d-7e8f9a0b1c2d","name":"Vendas"}`)

	_, _, err = client.OpenTicket(req, idemKey)
	require.Error(t, err)

	var clientErr *generic.ClientError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, 409, clientErr.StatusCode)
	assert.Equal(t, "ticket_already_open", clientErr.Code)
	assert.Equal(t, "An open ticket already exists", clientErr.Message)
	assert.Contains(t, string(clientErr.Details), "EXT-999")
}

func TestOpenTicketRaw(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-RAW","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
		},
	}))

	client := newTestClient()
	body := []byte(`{"id":"` + sampleTicketUUID + `","subject":"templated"}`)
	resp, trace, err := client.OpenTicketRaw(body, "open-raw-1")
	require.NoError(t, err)
	assert.Equal(t, "EXT-RAW", resp.ExternalID)
	assert.Equal(t, "open-raw-1", trace.Request.Header.Get("Idempotency-Key"))
	assert.Contains(t, string(trace.RequestTrace), `"id":"`+sampleTicketUUID+`"`)
	assert.Contains(t, string(trace.RequestTrace), `"subject":"templated"`)
	assert.NotContains(t, string(trace.RequestTrace), `"ticket_id":`)
}

func TestForwardMessage(t *testing.T) {
	resetGlobals(t)

	endpoint := testBaseURL + "/v1/tickets/" + sampleExternalID + "/messages"
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		endpoint: {
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `{"message_external_id":"external-msg-456","status":"received"}`),
		},
	}))

	client := newTestClient()
	req := &generic.MessageRequest{
		TicketID:   sampleTicketUUID,
		ExternalID: sampleExternalID,
		MessageID:  "msg-789",
		Direction:  "incoming",
		Sender:     generic.Sender{Type: "contact", ID: sampleContactUUID, Name: "João Silva"},
		Text:       "Olá, preciso de ajuda com meu pedido.",
		SentAt:     sentAt(),
	}
	idemKey := "forward-msg-789"

	_, _, err := client.ForwardMessage(sampleExternalID, req, idemKey)
	assert.EqualError(t, err, "unable to connect to server")

	resp, trace, err := client.ForwardMessage(sampleExternalID, req, idemKey)
	require.NoError(t, err)
	assert.Equal(t, "external-msg-456", resp.MessageExternalID)
	assert.Equal(t, "received", resp.Status)
	assert.Equal(t, idemKey, trace.Request.Header.Get("Idempotency-Key"))
	assert.Contains(t, string(trace.RequestTrace), `"direction":"incoming"`)
	assert.Contains(t, string(trace.RequestTrace), `"sender":{"type":"contact","id":"`+sampleContactUUID+`","name":"João Silva"}`)
}

func TestForwardMessageEscapesExternalID(t *testing.T) {
	resetGlobals(t)

	rawID := "EXT/with/slashes"
	endpoint := testBaseURL + "/v1/tickets/EXT%2Fwith%2Fslashes/messages"
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		endpoint: {
			httpx.NewMockResponse(200, nil, `{"message_external_id":"x","status":"received"}`),
		},
	}))

	client := newTestClient()
	req := &generic.MessageRequest{
		TicketID:   sampleTicketUUID,
		ExternalID: rawID,
		Direction:  "incoming",
		Sender:     generic.Sender{Type: "contact"},
		Text:       "hi",
		SentAt:     sentAt(),
	}

	_, _, err := client.ForwardMessage(rawID, req, "")
	require.NoError(t, err)
}

func TestCloseTicket(t *testing.T) {
	resetGlobals(t)

	endpoint := testBaseURL + "/v1/tickets/" + sampleExternalID + "/close"
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		endpoint: {
			httpx.NewMockResponse(200, nil, `{"status":"closed"}`),
			httpx.NewMockResponse(409, nil, `{"error":"ticket_already_closed","message":"Ticket is already closed"}`),
		},
	}))

	client := newTestClient()
	req := &generic.CloseRequest{
		TicketID:   sampleTicketUUID,
		ExternalID: sampleExternalID,
		ClosedBy:   generic.ActorRef{Type: "platform", ID: "system"},
		Reason:     "resolved",
		ClosedAt:   time.Date(2026, 5, 20, 14, 50, 0, 0, time.UTC),
	}
	idemKey := "close-" + sampleTicketUUID + "-evt1"

	trace, err := client.CloseTicket(sampleExternalID, req, idemKey)
	require.NoError(t, err)
	assert.Equal(t, idemKey, trace.Request.Header.Get("Idempotency-Key"))
	assert.Contains(t, string(trace.RequestTrace), `"closed_by":{"type":"platform","id":"system"}`)
	assert.Contains(t, string(trace.RequestTrace), `"reason":"resolved"`)

	_, err = client.CloseTicket(sampleExternalID, req, idemKey)
	require.Error(t, err)

	var clientErr *generic.ClientError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, 409, clientErr.StatusCode)
	assert.Equal(t, "ticket_already_closed", clientErr.Code)
}

func TestReopenTicket(t *testing.T) {
	resetGlobals(t)

	endpoint := testBaseURL + "/v1/tickets/" + sampleExternalID + "/reopen"
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		endpoint: {
			httpx.NewMockResponse(200, nil, `{"status":"open"}`),
			httpx.NewMockResponse(422, nil, `{"error":"reopen_not_supported","message":"This ticketer does not support ticket reopening"}`),
		},
	}))

	client := newTestClient()
	req := &generic.ReopenRequest{
		TicketID:   sampleTicketUUID,
		ExternalID: sampleExternalID,
		ReopenedBy: generic.ActorRef{Type: "platform", ID: "system"},
		ReopenedAt: time.Date(2026, 5, 20, 15, 5, 0, 0, time.UTC),
	}

	_, err := client.ReopenTicket(sampleExternalID, req, "reopen-"+sampleTicketUUID+"-evt1")
	require.NoError(t, err)

	_, err = client.ReopenTicket(sampleExternalID, req, "reopen-"+sampleTicketUUID+"-evt2")
	require.Error(t, err)

	var clientErr *generic.ClientError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, 422, clientErr.StatusCode)
	assert.Equal(t, "reopen_not_supported", clientErr.Code)
}

func TestSendHistory(t *testing.T) {
	resetGlobals(t)

	endpoint := testBaseURL + "/v1/tickets/" + sampleExternalID + "/history"
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		endpoint: {
			httpx.NewMockResponse(200, nil, `{"status":"history_received","messages_received":2}`),
		},
	}))

	client := newTestClient()
	req := &generic.HistoryRequest{
		TicketID:   sampleTicketUUID,
		ExternalID: sampleExternalID,
		Contact: generic.Contact{
			UUID: sampleContactUUID,
			Name: "João Silva",
			URN:  "whatsapp:+5511999999999",
		},
		Messages: []generic.HistoryMessage{
			{
				MessageID: "msg-001",
				Direction: "outgoing",
				Sender:    generic.Sender{Type: "bot"},
				Text:      "Olá! Como posso ajudar?",
				SentAt:    time.Date(2026, 5, 20, 14, 20, 0, 0, time.UTC),
			},
			{
				MessageID: "msg-002",
				Direction: "incoming",
				Sender:    generic.Sender{Type: "contact", ID: sampleContactUUID},
				Text:      "Quero falar com um atendente.",
				SentAt:    time.Date(2026, 5, 20, 14, 21, 0, 0, time.UTC),
			},
		},
	}

	trace, err := client.SendHistory(sampleExternalID, req, "")
	require.NoError(t, err)
	assert.Contains(t, string(trace.RequestTrace), `"messages":[`)
	assert.Contains(t, string(trace.RequestTrace), `"message_id":"msg-001"`)
	assert.Contains(t, string(trace.RequestTrace), `"message_id":"msg-002"`)
}

func TestRequestWithoutIdempotencyKey(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-1","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
		},
	}))

	client := newTestClient()
	req := &generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+551199"},
		OpenedAt: openedAt(),
	}

	_, trace, err := client.OpenTicket(req, "")
	require.NoError(t, err)
	assert.Empty(t, trace.Request.Header.Get("Idempotency-Key"))
}

func TestNewClientStripsTrailingSlash(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-1","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
		},
	}))

	client := generic.NewClient(http.DefaultClient, nil, testBaseURL+"/", testAPIToken)
	_, _, err := client.OpenTicket(&generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+551199"},
		OpenedAt: openedAt(),
	}, "")
	require.NoError(t, err)
}

func TestDefaultRoutes(t *testing.T) {
	routes := generic.DefaultRoutes()
	assert.Equal(t, "/v1/tickets", routes.OpenTicket)
	assert.Equal(t, "/v1/tickets/{external_id}/messages", routes.ForwardMessage)
	assert.Equal(t, "/v1/tickets/{external_id}/close", routes.CloseTicket)
	assert.Equal(t, "/v1/tickets/{external_id}/reopen", routes.ReopenTicket)
	assert.Equal(t, "/v1/tickets/{external_id}/history", routes.SendHistory)

	defaults := generic.DefaultRoutes()
	defaults.OpenTicket = "mutated"
	assert.Equal(t, "/v1/tickets", generic.DefaultRoutes().OpenTicket, "DefaultRoutes() must return a fresh copy")
}

func TestRoutesWithDefaults(t *testing.T) {
	merged := generic.Routes{OpenTicket: "/custom/open"}.WithDefaults(generic.DefaultRoutes())
	assert.Equal(t, "/custom/open", merged.OpenTicket)
	assert.Equal(t, "/v1/tickets/{external_id}/messages", merged.ForwardMessage)
	assert.Equal(t, "/v1/tickets/{external_id}/close", merged.CloseTicket)
}

func TestCustomRoutes(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/api/conversations": {
			httpx.NewMockResponse(201, nil, `{"external_id":"`+sampleExternalID+`","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
		},
		testBaseURL + "/api/conversations/" + sampleExternalID + "/inbound": {
			httpx.NewMockResponse(200, nil, `{"message_external_id":"x","status":"received"}`),
		},
		testBaseURL + "/api/conversations/" + sampleExternalID + "/close": {
			httpx.NewMockResponse(200, nil, `{"status":"closed"}`),
		},
		testBaseURL + "/api/conversations/" + sampleExternalID + "/reopen": {
			httpx.NewMockResponse(200, nil, `{"status":"open"}`),
		},
		testBaseURL + "/api/conversations/" + sampleExternalID + "/history": {
			httpx.NewMockResponse(200, nil, `{"status":"history_received","messages_received":0}`),
		},
	}))

	client := generic.NewClient(http.DefaultClient, nil, testBaseURL, testAPIToken, generic.WithRoutes(generic.Routes{
		OpenTicket:     "/api/conversations",
		ForwardMessage: "/api/conversations/{external_id}/inbound",
		CloseTicket:    "/api/conversations/{external_id}/close",
		ReopenTicket:   "/api/conversations/{external_id}/reopen",
		SendHistory:    "/api/conversations/{external_id}/history",
	}))

	contact := generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+551199"}

	_, _, err := client.OpenTicket(&generic.OpenRequest{TicketID: sampleTicketUUID, Contact: contact, OpenedAt: openedAt()}, "")
	require.NoError(t, err)

	_, _, err = client.ForwardMessage(sampleExternalID, &generic.MessageRequest{
		TicketID: sampleTicketUUID, ExternalID: sampleExternalID, Direction: "incoming",
		Sender: generic.Sender{Type: "contact"}, Text: "hi", SentAt: sentAt(),
	}, "")
	require.NoError(t, err)

	_, err = client.CloseTicket(sampleExternalID, &generic.CloseRequest{
		TicketID: sampleTicketUUID, ExternalID: sampleExternalID,
		ClosedBy: generic.ActorRef{Type: "platform"}, ClosedAt: openedAt(),
	}, "")
	require.NoError(t, err)

	_, err = client.ReopenTicket(sampleExternalID, &generic.ReopenRequest{
		TicketID: sampleTicketUUID, ExternalID: sampleExternalID,
		ReopenedBy: generic.ActorRef{Type: "platform"}, ReopenedAt: openedAt(),
	}, "")
	require.NoError(t, err)

	_, err = client.SendHistory(sampleExternalID, &generic.HistoryRequest{
		TicketID: sampleTicketUUID, ExternalID: sampleExternalID, Contact: contact, Messages: []generic.HistoryMessage{},
	}, "")
	require.NoError(t, err)
}

func TestCustomRoutesPartialOverride(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/api/conversations": {
			httpx.NewMockResponse(201, nil, `{"external_id":"`+sampleExternalID+`","status":"open","created_at":"2026-05-20T14:30:03Z"}`),
		},
		testBaseURL + "/v1/tickets/" + sampleExternalID + "/messages": {
			httpx.NewMockResponse(200, nil, `{"message_external_id":"x","status":"received"}`),
		},
	}))

	client := generic.NewClient(http.DefaultClient, nil, testBaseURL, testAPIToken, generic.WithRoutes(generic.Routes{
		OpenTicket: "/api/conversations",
	}))

	contact := generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+551199"}

	_, _, err := client.OpenTicket(&generic.OpenRequest{TicketID: sampleTicketUUID, Contact: contact, OpenedAt: openedAt()}, "")
	require.NoError(t, err, "custom OpenTicket route must be used")

	_, _, err = client.ForwardMessage(sampleExternalID, &generic.MessageRequest{
		TicketID: sampleTicketUUID, ExternalID: sampleExternalID, Direction: "incoming",
		Sender: generic.Sender{Type: "contact"}, Text: "hi", SentAt: sentAt(),
	}, "")
	require.NoError(t, err, "ForwardMessage must fall back to DefaultRoutes when not overridden")
}

func TestClientErrorOnMalformedErrorBody(t *testing.T) {
	resetGlobals(t)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		testBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(500, nil, `not json`),
		},
	}))

	client := newTestClient()
	_, _, err := client.OpenTicket(&generic.OpenRequest{
		TicketID: sampleTicketUUID,
		Contact:  generic.Contact{UUID: sampleContactUUID, URN: "whatsapp:+551199"},
		OpenedAt: openedAt(),
	}, "")
	require.Error(t, err)

	var clientErr *generic.ClientError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, 500, clientErr.StatusCode)
	assert.Equal(t, "HTTP 500", clientErr.Error())
}
