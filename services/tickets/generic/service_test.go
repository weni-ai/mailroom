package generic_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/services/tickets/generic"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/null"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	svcBaseURL       = "https://partner.example.com"
	svcAPIToken      = "svc-token"
	svcWebhookSecret = "svc-webhook-secret"
)

// newTicketer returns an in-memory flow ticketer registered as the "generic"
// type. The UUID is fixed so the webhook_base_url metadata field is stable
// across tests.
func newTicketer() *flows.Ticketer {
	return flows.NewTicketer(static.NewTicketer(
		assets.TicketerUUID("11111111-2222-3333-4444-555555555555"),
		"Generic Partner",
		"generic",
	))
}

// newDefaultTopic constructs an in-memory General topic so tests don't need a
// fresh OrgAssets DB query (which depends on the test DB schema being in
// sync with the Go code).
func newDefaultTopic() *flows.Topic {
	return flows.NewTopic(static.NewTopic(
		assets.TopicUUID(testdata.DefaultTopic.UUID),
		"General",
		assets.QueueUUID(""),
	))
}

func newModelTicketer(config map[string]string) *models.Ticketer {
	return models.BuildTicketer(
		models.TicketerID(1),
		assets.TicketerUUID("11111111-2222-3333-4444-555555555555"),
		testdata.Org1.ID,
		"generic",
		"Generic Partner",
		config,
	)
}

func TestNewServiceConfigValidation(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	ticketer := newTicketer()

	cases := []struct {
		name   string
		config map[string]string
	}{
		{"empty config", map[string]string{}},
		{"missing api_token", map[string]string{"base_url": svcBaseURL, "webhook_secret": svcWebhookSecret}},
		{"missing webhook_secret", map[string]string{"base_url": svcBaseURL, "api_token": svcAPIToken}},
		{"missing base_url", map[string]string{"api_token": svcAPIToken, "webhook_secret": svcWebhookSecret}},
		{"whitespace base_url", map[string]string{"base_url": "   ", "api_token": svcAPIToken, "webhook_secret": svcWebhookSecret}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(tc.config), context.Background(), nil)
			assert.EqualError(t, err, "missing base_url, api_token or webhook_secret in generic ticketer config")
		})
	}

	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
	}), context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc, err = generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":          svcBaseURL,
		"api_token":         svcAPIToken,
		"skip_webhook_hmac": "true",
	}), context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, svc)

	_, err = generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
		"open_template":  `{"id":"{{.ticket_id"`,
	}), context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid open_template")

	_, err = generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":               svcBaseURL,
		"api_token":              svcAPIToken,
		"webhook_secret":         svcWebhookSecret,
		"open_response_template": `{"external_id":"{{.id"`,
	}), context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid open_response_template")

	_, err = generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":         svcBaseURL,
		"api_token":        svcAPIToken,
		"webhook_secret":   svcWebhookSecret,
		"forward_template": `{"text":"{{.text"`,
	}), context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid forward_template")

	_, err = generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":                  svcBaseURL,
		"api_token":                 svcAPIToken,
		"webhook_secret":            svcWebhookSecret,
		"forward_response_template": `{"message_external_id":"{{.id"`,
	}), context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid forward_response_template")
}

func TestOpenAndForward(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	originalDomain := rt.Config.Domain
	rt.Config.Domain = "mailroom.test"
	defer func() { rt.Config.Domain = originalDomain }()

	// CreateTestSession itself dispatches webhooks via the default requestor,
	// so it must run before we install the mock requestor below.
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-OPEN-1","status":"open","created_at":"2026-05-20T14:30:01Z"}`),
		},
		svcBaseURL + "/v1/tickets/EXT-OPEN-1/messages": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(202, nil, `{"message_external_id":"MSG-EXT-1","status":"queued"}`),
			httpx.NewMockResponse(202, nil, `{"message_external_id":"MSG-EXT-2","status":"queued"}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
		"project_uuid":   "f0e1d2c3-b4a5-4968-8c7d-9e0f1a2b3c4d",
		"project_name":   "Partner Project",
	}), context.Background(), nil)
	require.NoError(t, err)

	defaultTopic := newDefaultTopic()

	// Open: first attempt returns a connection error and surfaces it.
	logger := &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Need human help", nil, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to connect to server")
	assert.Equal(t, 1, len(logger.Logs))

	// Open: second attempt succeeds and external_id is set on the ticket.
	logger = &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, "Need human help", nil, logger.Log)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, "Need human help", ticket.Body())
	assert.Equal(t, "EXT-OPEN-1", ticket.ExternalID())
	require.Equal(t, 1, len(logger.Logs))

	openLog := logger.Logs[0]
	assert.Contains(t, openLog.Request, "POST /v1/tickets ")
	assert.Contains(t, openLog.Request, "Authorization: Bearer ****************")
	// Go normalizes header names — "X-API-Version" hits the wire as
	// "X-Api-Version".
	assert.Contains(t, openLog.Request, "X-Api-Version: 1")
	assert.Contains(t, openLog.Request, "Idempotency-Key: open-"+string(ticket.UUID()))
	assert.Contains(t, openLog.Request, `"ticket_id":"`+string(ticket.UUID())+`"`)
	assert.Contains(t, openLog.Request, `"body":"Need human help"`)
	assert.Contains(t, openLog.Request, `"project_uuid":"f0e1d2c3-b4a5-4968-8c7d-9e0f1a2b3c4d"`)
	assert.Contains(t, openLog.Request, `"project_name":"Partner Project"`)
	assert.Contains(t, openLog.Request, `"webhook_base_url":"https://mailroom.test/mr/tickets/types/generic/event_callback/11111111-2222-3333-4444-555555555555"`)

	// Forward: connection error
	dbTicket := models.NewTicket(
		ticket.UUID(),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.RocketChat.ID, // borrow any existing ticketer id; not persisted
		"EXT-OPEN-1",
		testdata.DefaultTopic.ID,
		"Need human help",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid":    string(testdata.Cathy.UUID),
			"contact-display": "Cathy",
		},
	)

	logger = &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello!", nil, nil, null.NullString, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to connect to server")

	// Forward: text-only happy path
	logger = &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello!", nil, nil, null.NullString, logger.Log)
	require.NoError(t, err)
	require.Equal(t, 1, len(logger.Logs))

	fwdLog := logger.Logs[0]
	assert.Contains(t, fwdLog.Request, "POST /v1/tickets/EXT-OPEN-1/messages ")
	assert.Contains(t, fwdLog.Request, "Idempotency-Key: forward-4fa340ae-1fb0-4666-98db-2177fe9bf31c")
	assert.Contains(t, fwdLog.Request, `"direction":"incoming"`)
	assert.Contains(t, fwdLog.Request, `"sender":{"type":"contact","id":"`+string(testdata.Cathy.UUID)+`","name":"Cathy"}`)
	assert.Contains(t, fwdLog.Request, `"text":"Hello!"`)
	assert.NotContains(t, fwdLog.Request, `"attachments":`)

	// Forward: with attachments
	logger = &flows.HTTPLogger{}
	attachments := []utils.Attachment{
		"image/jpg:https://link.to/image.jpg",
		"audio/ogg:https://link.to/audio.ogg",
	}
	err = svc.Forward(dbTicket, flows.MsgUUID("c0c0c0c0-d0d0-e0e0-f0f0-010203040506"), "with attach", attachments, nil, null.String("EXT-MSG-99"), logger.Log)
	require.NoError(t, err)
	require.Equal(t, 1, len(logger.Logs))

	fwdLog = logger.Logs[0]
	assert.Contains(t, fwdLog.Request, `"text":"with attach"`)
	assert.Contains(t, fwdLog.Request, `"url":"https://link.to/image.jpg"`)
	assert.Contains(t, fwdLog.Request, `"content_type":"image/jpg"`)
	assert.Contains(t, fwdLog.Request, `"url":"https://link.to/audio.ogg"`)
	assert.Contains(t, fwdLog.Request, `"source_message_external_id":"EXT-MSG-99"`)
}

func TestOpenAlreadyOpenIsTreatedAsSuccess(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(54321))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(409, nil, `{"error":"ticket_already_open","message":"already exists","details":{"external_id":"EXT-EXIST-7"}}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
	}), context.Background(), nil)
	require.NoError(t, err)

	defaultTopic := newDefaultTopic()

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, "Need human help", nil, logger.Log)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "EXT-EXIST-7", ticket.ExternalID())
}

func TestOpenPartnerError(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(11111))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(422, nil, `{"error":"invalid_topic","message":"unknown topic uuid"}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
	}), context.Background(), nil)
	require.NoError(t, err)

	defaultTopic := newDefaultTopic()

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, "Need human help", nil, logger.Log)
	assert.Nil(t, ticket)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error opening generic ticket")

	var clientErr *generic.ClientError
	require.True(t, errors.As(err, &clientErr))
	assert.Equal(t, 422, clientErr.StatusCode)
	assert.Equal(t, "invalid_topic", clientErr.Code)
}

func TestCloseAndReopen(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(99999))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets/EXT-A/close": {
			httpx.NewMockResponse(204, nil, ``),
		},
		svcBaseURL + "/v1/tickets/EXT-B/close": {
			httpx.NewMockResponse(409, nil, `{"error":"already_closed","message":"already closed"}`),
		},
		svcBaseURL + "/v1/tickets/EXT-FAIL/close": {
			httpx.NewMockResponse(500, nil, `{"error":"server_error","message":"boom"}`),
		},
		svcBaseURL + "/v1/tickets/EXT-A/reopen": {
			httpx.NewMockResponse(200, nil, ``),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
	}), context.Background(), nil)
	require.NoError(t, err)

	ticketA := models.NewTicket("88bfa1dc-be33-45c2-b469-294ecb0eba90", testdata.Org1.ID, testdata.Cathy.ID, testdata.RocketChat.ID, "EXT-A", testdata.DefaultTopic.ID, "first", models.NilUserID, nil)
	ticketB := models.NewTicket("645eee60-7e84-4a9e-ade3-4fce01ae28f1", testdata.Org1.ID, testdata.Bob.ID, testdata.RocketChat.ID, "EXT-B", testdata.DefaultTopic.ID, "second", models.NilUserID, nil)
	ticketEmpty := models.NewTicket("8aa8c4bc-9b94-4e3c-9bfe-3bb4b9ea3c61", testdata.Org1.ID, testdata.Bob.ID, testdata.RocketChat.ID, "", testdata.DefaultTopic.ID, "no-ext", models.NilUserID, nil)

	// Close: a happy 204 + a 409 (already closed, ignored) + a ticket with no
	// external_id (skipped silently).
	logger := &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticketA, ticketB, ticketEmpty}, logger.Log)
	require.NoError(t, err)
	assert.Equal(t, 2, len(logger.Logs)) // empty external_id skips the call

	assert.Contains(t, logger.Logs[0].Request, "POST /v1/tickets/EXT-A/close ")
	assert.Contains(t, logger.Logs[0].Request, `"closed_by":{"type":"platform","id":"system"}`)
	assert.Contains(t, logger.Logs[1].Request, "POST /v1/tickets/EXT-B/close ")

	// Close: a 500 should bubble up as an error after logging.
	ticketFail := models.NewTicket("0f0f0f0f-1111-2222-3333-444444444444", testdata.Org1.ID, testdata.Bob.ID, testdata.RocketChat.ID, "EXT-FAIL", testdata.DefaultTopic.ID, "bad", models.NilUserID, nil)
	logger = &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticketFail}, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error closing generic ticket")
	assert.Equal(t, 1, len(logger.Logs))

	// Reopen: happy path
	logger = &flows.HTTPLogger{}
	err = svc.Reopen([]*models.Ticket{ticketA}, logger.Log)
	require.NoError(t, err)
	require.Equal(t, 1, len(logger.Logs))
	assert.Contains(t, logger.Logs[0].Request, "POST /v1/tickets/EXT-A/reopen ")
	assert.Contains(t, logger.Logs[0].Request, `"reopened_by":{"type":"platform","id":"system"}`)
}

func TestSendHistoryIsNoop(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(nil))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
	}), context.Background(), nil)
	require.NoError(t, err)

	dbTicket := models.NewTicket("88bfa1dc-be33-45c2-b469-294ecb0eba90", testdata.Org1.ID, testdata.Cathy.ID, testdata.RocketChat.ID, "EXT-X", testdata.DefaultTopic.ID, "x", models.NilUserID, nil)
	logger := &flows.HTTPLogger{}
	err = svc.SendHistory(dbTicket, testdata.Cathy.ID, nil, logger.Log)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(logger.Logs))
}

func TestCustomRoutesThroughConfig(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(22222))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/api/conversations": {
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-ROUTE-1","status":"open","created_at":"2026-05-20T14:30:00Z"}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
		"route_open":     "/api/conversations",
	}), context.Background(), nil)
	require.NoError(t, err)

	defaultTopic := newDefaultTopic()

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, "test", nil, logger.Log)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "EXT-ROUTE-1", ticket.ExternalID())
	require.Equal(t, 1, len(logger.Logs))
	assert.Contains(t, logger.Logs[0].Request, "POST /api/conversations ")
}

func TestOpenWithTemplate(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(33333))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"external_id":"EXT-TMPL-1","status":"open","created_at":"2026-05-20T14:30:00Z"}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
		"open_template":  `{"id":"{{.ticket_id}}","customer":{{json .contact}},"subject":"{{.body}}"}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, newDefaultTopic(), "Need human help", nil, logger.Log)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "EXT-TMPL-1", ticket.ExternalID())
	require.Equal(t, 1, len(logger.Logs))

	reqBody := logger.Logs[0].Request
	assert.Contains(t, reqBody, `"id":"`+string(ticket.UUID())+`"`)
	assert.Contains(t, reqBody, `"subject":"Need human help"`)
	assert.Contains(t, reqBody, `"customer":`)
	assert.NotContains(t, reqBody, `"ticket_id":`)
}

func TestOpenWithTemplateInvalidJSON(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(44444))
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":       svcBaseURL,
		"api_token":      svcAPIToken,
		"webhook_secret": svcWebhookSecret,
		"open_template":  `not-json {{.ticket_id}}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	_, err = svc.Open(session, newDefaultTopic(), "test", nil, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open_template")
	assert.Contains(t, err.Error(), "invalid JSON")
	assert.Equal(t, 0, len(logger.Logs))
}

func TestOpenWithResponseTemplate(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(55555))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"data":{"id":"EXT-MAPPED-1","state":"open","created":"2026-05-20T14:30:03Z"}}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":               svcBaseURL,
		"api_token":              svcAPIToken,
		"webhook_secret":         svcWebhookSecret,
		"open_template":          `{"id":"{{.ticket_id}}","subject":"{{.body}}"}`,
		"open_response_template": `{"external_id":"{{.data.id}}","status":"{{.data.state}}","created_at":"{{.data.created}}"}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, newDefaultTopic(), "Need help", nil, logger.Log)
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "EXT-MAPPED-1", ticket.ExternalID())
	require.Equal(t, 1, len(logger.Logs))
	assert.Contains(t, logger.Logs[0].Request, `"subject":"Need help"`)
}

func TestOpenWithResponseTemplateMissingExternalID(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(66666))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets": {
			httpx.NewMockResponse(201, nil, `{"data":{"id":""}}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":               svcBaseURL,
		"api_token":              svcAPIToken,
		"webhook_secret":         svcWebhookSecret,
		"open_response_template": `{"external_id":"{{.data.id}}","status":"open"}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	_, err = svc.Open(session, newDefaultTopic(), "test", nil, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not return an external_id")
}

func TestForwardWithTemplate(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(77777))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets/EXT-FWD-1/messages": {
			httpx.NewMockResponse(200, nil, `{"message_external_id":"MSG-TMPL-1","status":"received"}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":         svcBaseURL,
		"api_token":        svcAPIToken,
		"webhook_secret":   svcWebhookSecret,
		"forward_template": `{"ticket":"{{.external_id}}","from":{{json .sender}},"body":"{{.text}}","msg_id":"{{.message_id}}"}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	dbTicket := models.NewTicket(
		flows.TicketUUID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.RocketChat.ID,
		"EXT-FWD-1",
		testdata.DefaultTopic.ID,
		"body",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid":    string(testdata.Cathy.UUID),
			"contact-display": "Cathy",
		},
	)

	logger := &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello templated!", nil, nil, null.NullString, logger.Log)
	require.NoError(t, err)
	require.Equal(t, 1, len(logger.Logs))

	reqBody := logger.Logs[0].Request
	assert.Contains(t, reqBody, `"ticket":"EXT-FWD-1"`)
	assert.Contains(t, reqBody, `"body":"Hello templated!"`)
	assert.Contains(t, reqBody, `"msg_id":"4fa340ae-1fb0-4666-98db-2177fe9bf31c"`)
	assert.Contains(t, reqBody, `"from":`)
	assert.NotContains(t, reqBody, `"ticket_id":`)
	assert.NotContains(t, reqBody, `"direction":`)
}

func TestForwardWithTemplateInvalidJSON(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":         svcBaseURL,
		"api_token":        svcAPIToken,
		"webhook_secret":   svcWebhookSecret,
		"forward_template": `not-json {{.text}}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	dbTicket := models.NewTicket(
		flows.TicketUUID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.RocketChat.ID,
		"EXT-FWD-1",
		testdata.DefaultTopic.ID,
		"body",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid":    string(testdata.Cathy.UUID),
			"contact-display": "Cathy",
		},
	)

	logger := &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello", nil, nil, null.NullString, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward_template")
	assert.Contains(t, err.Error(), "invalid JSON")
	assert.Equal(t, 0, len(logger.Logs))
}

func TestForwardWithResponseTemplate(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)))

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets/EXT-FWD-2/messages": {
			httpx.NewMockResponse(200, nil, `{"result":{"id":"MSG-MAPPED-1","state":"received"}}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":                  svcBaseURL,
		"api_token":                 svcAPIToken,
		"webhook_secret":            svcWebhookSecret,
		"forward_template":          `{"ticket":"{{.external_id}}","body":"{{.text}}"}`,
		"forward_response_template": `{"message_external_id":"{{.result.id}}","status":"{{.result.state}}"}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	dbTicket := models.NewTicket(
		flows.TicketUUID("bbbbbbbb-cccc-dddd-eeee-ffffffffffff"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.RocketChat.ID,
		"EXT-FWD-2",
		testdata.DefaultTopic.ID,
		"body",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid":    string(testdata.Cathy.UUID),
			"contact-display": "Cathy",
		},
	)

	logger := &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello mapped!", nil, nil, null.NullString, logger.Log)
	require.NoError(t, err)
	require.Equal(t, 1, len(logger.Logs))
	assert.Contains(t, logger.Logs[0].Request, `"body":"Hello mapped!"`)
}

func TestForwardWithResponseTemplateInvalidJSON(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		svcBaseURL + "/v1/tickets/EXT-FWD-3/messages": {
			httpx.NewMockResponse(200, nil, `{"result":{"id":"MSG-1"}}`),
		},
	}))

	ticketer := newTicketer()
	svc, err := generic.NewService(rt.Config, http.DefaultClient, nil, ticketer, newModelTicketer(map[string]string{
		"base_url":                  svcBaseURL,
		"api_token":                 svcAPIToken,
		"webhook_secret":            svcWebhookSecret,
		"forward_response_template": `not-json {{.result.id}}`,
	}), context.Background(), nil)
	require.NoError(t, err)

	dbTicket := models.NewTicket(
		flows.TicketUUID("cccccccc-dddd-eeee-ffff-000000000000"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.RocketChat.ID,
		"EXT-FWD-3",
		testdata.DefaultTopic.ID,
		"body",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid":    string(testdata.Cathy.UUID),
			"contact-display": "Cathy",
		},
	)

	logger := &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "Hello", nil, nil, null.NullString, logger.Log)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward_response_template")
	assert.Contains(t, err.Error(), "invalid JSON")
	require.Equal(t, 1, len(logger.Logs))
}
