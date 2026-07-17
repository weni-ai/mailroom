package generic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/pkg/errors"
)

// apiVersion is the contract version emitted in the X-API-Version header on
// every platform-to-ticketer request. See docs/generic-ticketer-service.md.
const apiVersion = "1"

// externalIDPlaceholder is the token replaced in route templates by the
// URL-escaped external_id of the ticket at request time.
const externalIDPlaceholder = "{external_id}"

// Routes lets callers override per-endpoint path templates on the partner
// side. Empty fields are filled in by WithDefaults, so callers can override
// just the routes they need. Templates may contain the {external_id}
// placeholder, which is URL-escaped at request time.
type Routes struct {
	OpenTicket     string
	ForwardMessage string
	CloseTicket    string
	ReopenTicket   string
	SendHistory    string
}

// DefaultRoutes returns the opinionated route templates that match the
// generic ticketer contract documented in docs/generic-ticketer-service.md.
func DefaultRoutes() Routes {
	return Routes{
		OpenTicket:     "/v1/tickets",
		ForwardMessage: "/v1/tickets/" + externalIDPlaceholder + "/messages",
		CloseTicket:    "/v1/tickets/" + externalIDPlaceholder + "/close",
		ReopenTicket:   "/v1/tickets/" + externalIDPlaceholder + "/reopen",
		SendHistory:    "/v1/tickets/" + externalIDPlaceholder + "/history",
	}
}

// WithDefaults returns a copy of r where empty fields are filled in from d.
func (r Routes) WithDefaults(d Routes) Routes {
	if r.OpenTicket == "" {
		r.OpenTicket = d.OpenTicket
	}
	if r.ForwardMessage == "" {
		r.ForwardMessage = d.ForwardMessage
	}
	if r.CloseTicket == "" {
		r.CloseTicket = d.CloseTicket
	}
	if r.ReopenTicket == "" {
		r.ReopenTicket = d.ReopenTicket
	}
	if r.SendHistory == "" {
		r.SendHistory = d.SendHistory
	}
	return r
}

// ClientOption configures optional behavior on a Client at construction.
type ClientOption func(*Client)

// WithRoutes overrides one or more route templates. Empty fields fall back to
// DefaultRoutes, so callers can supply only the routes they need to customize.
// Should be passed at most once to NewClient — subsequent calls replace the
// previous override.
func WithRoutes(r Routes) ClientOption {
	return func(c *Client) {
		c.routes = r.WithDefaults(DefaultRoutes())
	}
}

// Client is the HTTP client used by the generic ticketer service to call the
// partner endpoints documented in docs/generic-ticketer-service.md.
type Client struct {
	httpClient  *http.Client
	httpRetries *httpx.RetryConfig
	baseURL     string
	apiToken    string
	routes      Routes
}

// NewClient builds a Client targeting the given partner base URL using the
// provided bearer token for authentication. Trailing slashes in baseURL are
// stripped. Route templates default to DefaultRoutes; pass WithRoutes to
// override them.
func NewClient(httpClient *http.Client, httpRetries *httpx.RetryConfig, baseURL, apiToken string, opts ...ClientOption) *Client {
	c := &Client{
		httpClient:  httpClient,
		httpRetries: httpRetries,
		baseURL:     strings.TrimRight(baseURL, "/"),
		apiToken:    apiToken,
		routes:      DefaultRoutes(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// endpoint substitutes {external_id} in a route template with the URL-escaped
// value, returning the resulting path.
func (c *Client) endpoint(template, externalID string) string {
	if !strings.Contains(template, externalIDPlaceholder) {
		return template
	}
	return strings.ReplaceAll(template, externalIDPlaceholder, url.PathEscape(externalID))
}

// ClientError is returned when the partner answers with a 4xx or 5xx status
// code. It maps to the error envelope documented in section 9 of the spec
// ({error, message, details}).
type ClientError struct {
	StatusCode int             `json:"-"`
	Code       string          `json:"error"`
	Message    string          `json:"message"`
	Details    json.RawMessage `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *ClientError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// Shared DTOs --------------------------------------------------------------

// Topic identifies the queue/subject of the ticket.
type Topic struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name,omitempty"`
	QueueUUID string `json:"queue_uuid,omitempty"`
}

// Contact carries the contact data sent on open and on history.
type Contact struct {
	UUID     string   `json:"uuid"`
	Name     string   `json:"name,omitempty"`
	URN      string   `json:"urn"`
	URNs     []string `json:"urns,omitempty"`
	Language string   `json:"language,omitempty"`
}

// Assignee is the agent suggested at ticket open.
type Assignee struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	UUID  string `json:"uuid,omitempty"`
}

// Sender identifies who originated a message.
type Sender struct {
	Type  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// Attachment represents a single message attachment.
type Attachment struct {
	ID          string                 `json:"id,omitempty"`
	URL         string                 `json:"url"`
	ContentType string                 `json:"content_type,omitempty"`
	Filename    string                 `json:"filename,omitempty"`
	Size        int64                  `json:"size,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ActorRef identifies the actor behind a state change (close/reopen).
type ActorRef struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Open ---------------------------------------------------------------------

// OpenRequest is the body of POST /v1/tickets.
type OpenRequest struct {
	TicketID string                 `json:"ticket_id"`
	Topic    *Topic                 `json:"topic,omitempty"`
	Contact  Contact                `json:"contact"`
	Body     string                 `json:"body,omitempty"`
	Assignee *Assignee              `json:"assignee,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	OpenedAt time.Time              `json:"opened_at"`
}

// OpenResponse is the partner reply to POST /v1/tickets.
type OpenResponse struct {
	ExternalID string    `json:"external_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// OpenTicket creates a new ticket on the partner side.
func (c *Client) OpenTicket(req *OpenRequest, idempotencyKey string) (*OpenResponse, *httpx.Trace, error) {
	trace, err := c.openTicketRequest(req, idempotencyKey)
	if err != nil {
		return nil, trace, err
	}
	resp, err := decodeOpenResponse(trace.ResponseBody)
	return resp, trace, err
}

// OpenTicketRaw creates a new ticket using a pre-rendered JSON body (e.g. from
// open_template). The partner response is parsed as the standard OpenResponse
// envelope unless the caller maps it separately.
func (c *Client) OpenTicketRaw(body []byte, idempotencyKey string) (*OpenResponse, *httpx.Trace, error) {
	trace, err := c.openTicketRequest(json.RawMessage(body), idempotencyKey)
	if err != nil {
		return nil, trace, err
	}
	resp, err := decodeOpenResponse(trace.ResponseBody)
	return resp, trace, err
}

// openTicketRequest performs the Open HTTP call without parsing the response
// body, so callers can apply open_response_template or the default decoder.
func (c *Client) openTicketRequest(payload interface{}, idempotencyKey string) (*httpx.Trace, error) {
	return c.request(http.MethodPost, c.endpoint(c.routes.OpenTicket, ""), payload, nil, idempotencyKey)
}

// Forward (incoming message) -----------------------------------------------

// MessageRequest is the body of POST /v1/tickets/{external_id}/messages.
type MessageRequest struct {
	TicketID    string                 `json:"ticket_id"`
	ExternalID  string                 `json:"external_id"`
	MessageID   string                 `json:"message_id,omitempty"`
	Direction   string                 `json:"direction"`
	Sender      Sender                 `json:"sender"`
	Text        string                 `json:"text,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	SentAt      time.Time              `json:"sent_at"`
}

// MessageResponse is the partner reply to POST /v1/tickets/{external_id}/messages.
type MessageResponse struct {
	MessageExternalID string `json:"message_external_id"`
	Status            string `json:"status"`
}

// ForwardMessage delivers an incoming message from the contact to the partner.
func (c *Client) ForwardMessage(externalID string, req *MessageRequest, idempotencyKey string) (*MessageResponse, *httpx.Trace, error) {
	trace, err := c.forwardMessageRequest(externalID, req, idempotencyKey)
	if err != nil {
		return nil, trace, err
	}
	resp, err := decodeForwardResponse(trace.ResponseBody)
	return resp, trace, err
}

// ForwardMessageRaw delivers an incoming message using a pre-rendered JSON body
// (e.g. from forward_template). The partner response is parsed as the standard
// MessageResponse envelope unless the caller maps it separately.
func (c *Client) ForwardMessageRaw(externalID string, body []byte, idempotencyKey string) (*MessageResponse, *httpx.Trace, error) {
	trace, err := c.forwardMessageRequest(externalID, json.RawMessage(body), idempotencyKey)
	if err != nil {
		return nil, trace, err
	}
	resp, err := decodeForwardResponse(trace.ResponseBody)
	return resp, trace, err
}

// forwardMessageRequest performs the Forward HTTP call without parsing the
// response body, so callers can apply forward_response_template or the default
// decoder.
func (c *Client) forwardMessageRequest(externalID string, payload interface{}, idempotencyKey string) (*httpx.Trace, error) {
	return c.request(http.MethodPost, c.endpoint(c.routes.ForwardMessage, externalID), payload, nil, idempotencyKey)
}

// Close --------------------------------------------------------------------

// CloseRequest is the body of POST /v1/tickets/{external_id}/close.
type CloseRequest struct {
	TicketID   string                 `json:"ticket_id"`
	ExternalID string                 `json:"external_id"`
	ClosedBy   ActorRef               `json:"closed_by"`
	Reason     string                 `json:"reason,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ClosedAt   time.Time              `json:"closed_at"`
}

// CloseTicket notifies the partner that the ticket was closed on the platform.
func (c *Client) CloseTicket(externalID string, req *CloseRequest, idempotencyKey string) (*httpx.Trace, error) {
	return c.closeTicketRequest(externalID, req, idempotencyKey)
}

// CloseTicketRaw notifies the partner of a ticket close using a pre-rendered
// JSON body (e.g. from close_template).
func (c *Client) CloseTicketRaw(externalID string, body []byte, idempotencyKey string) (*httpx.Trace, error) {
	return c.closeTicketRequest(externalID, json.RawMessage(body), idempotencyKey)
}

func (c *Client) closeTicketRequest(externalID string, payload interface{}, idempotencyKey string) (*httpx.Trace, error) {
	return c.request(http.MethodPost, c.endpoint(c.routes.CloseTicket, externalID), payload, nil, idempotencyKey)
}

// Reopen -------------------------------------------------------------------

// ReopenRequest is the body of POST /v1/tickets/{external_id}/reopen.
type ReopenRequest struct {
	TicketID   string                 `json:"ticket_id"`
	ExternalID string                 `json:"external_id"`
	ReopenedBy ActorRef               `json:"reopened_by"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	ReopenedAt time.Time              `json:"reopened_at"`
}

// ReopenTicket notifies the partner that the ticket was reopened on the platform.
func (c *Client) ReopenTicket(externalID string, req *ReopenRequest, idempotencyKey string) (*httpx.Trace, error) {
	return c.request(http.MethodPost, c.endpoint(c.routes.ReopenTicket, externalID), req, nil, idempotencyKey)
}

// History ------------------------------------------------------------------

// HistoryMessage is one entry in the conversation history payload.
type HistoryMessage struct {
	MessageID   string                 `json:"message_id,omitempty"`
	Direction   string                 `json:"direction"`
	Sender      Sender                 `json:"sender"`
	Text        string                 `json:"text,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	SentAt      time.Time              `json:"sent_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HistoryRequest is the body of POST /v1/tickets/{external_id}/history.
type HistoryRequest struct {
	TicketID   string                 `json:"ticket_id"`
	ExternalID string                 `json:"external_id"`
	Contact    Contact                `json:"contact"`
	Messages   []HistoryMessage       `json:"messages"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// SendHistory delivers the conversation history that preceded the ticket open.
func (c *Client) SendHistory(externalID string, req *HistoryRequest, idempotencyKey string) (*httpx.Trace, error) {
	return c.request(http.MethodPost, c.endpoint(c.routes.SendHistory, externalID), req, nil, idempotencyKey)
}

// Internal -----------------------------------------------------------------

func (c *Client) request(method, endpoint string, payload, response interface{}, idempotencyKey string) (*httpx.Trace, error) {
	fullURL := c.baseURL + endpoint

	headers := map[string]string{
		"Authorization": "Bearer " + c.apiToken,
		"Content-Type":  "application/json",
		"X-API-Version": apiVersion,
		"X-Request-Id":  string(uuids.New()),
	}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}

	var body io.Reader
	if payload != nil {
		switch p := payload.(type) {
		case []byte:
			body = bytes.NewReader(p)
		case json.RawMessage:
			body = bytes.NewReader(p)
		default:
			data, err := jsonx.Marshal(payload)
			if err != nil {
				return nil, errors.Wrap(err, "error marshalling request payload")
			}
			body = bytes.NewReader(data)
		}
	}

	req, err := httpx.NewRequest(method, fullURL, body, headers)
	if err != nil {
		return nil, err
	}

	trace, err := httpx.DoTrace(c.httpClient, req, c.httpRetries, nil, -1)
	if err != nil {
		return trace, err
	}

	if trace.Response.StatusCode >= 400 {
		clientErr := &ClientError{StatusCode: trace.Response.StatusCode}
		if len(trace.ResponseBody) > 0 {
			_ = jsonx.Unmarshal(trace.ResponseBody, clientErr)
		}
		if clientErr.Code == "" {
			clientErr.Message = fmt.Sprintf("HTTP %d", trace.Response.StatusCode)
		}
		return trace, clientErr
	}

	if response != nil && len(trace.ResponseBody) > 0 {
		if err := jsonx.Unmarshal(trace.ResponseBody, response); err != nil {
			return trace, errors.Wrap(err, "error unmarshalling response")
		}
	}

	return trace, nil
}
