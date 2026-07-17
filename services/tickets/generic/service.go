// Package generic implements a generic HTTP-based ticketer service.
//
// The contract spoken by this ticketer is documented in
// docs/generic-ticketer-service.md. Any partner that implements the documented
// HTTP endpoints can plug into the platform as a ticketer without requiring
// custom code in Mailroom.
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

const (
	typeGeneric = "generic"

	// Required config
	configBaseURL         = "base_url"
	configAPIToken        = "api_token"
	configWebhookSecret   = "webhook_secret"
	configSkipWebhookHMAC = "skip_webhook_hmac"

	// Optional metadata enrichment for partner-side context
	configProjectUUID = "project_uuid"
	configProjectName = "project_name"

	// Optional per-endpoint route overrides. When empty, DefaultRoutes is used.
	configRouteOpen    = "route_open"
	configRouteForward = "route_forward"
	configRouteClose   = "route_close"
	configRouteReopen  = "route_reopen"
	configRouteHistory = "route_history"

	// Optional Go text/template that renders the Open request body. When empty,
	// the standard OpenRequest JSON contract is sent.
	configOpenTemplate = "open_template"

	// Optional Go text/template that maps the partner Open response JSON into
	// the standard OpenResponse shape (external_id, status, created_at).
	configOpenResponseTemplate = "open_response_template"

	// Optional Go text/template that renders the Forward request body. When
	// empty, the standard MessageRequest JSON contract is sent.
	configForwardTemplate = "forward_template"

	// Optional Go text/template that maps the partner Forward response JSON
	// into the standard MessageResponse shape (message_external_id, status).
	configForwardResponseTemplate = "forward_response_template"

	// Optional Go text/template that renders the Close request body. When
	// empty, the standard CloseRequest JSON contract is sent.
	configCloseTemplate = "close_template"

	// Optional Go text/template that maps the partner Close response JSON
	// into the standard CloseResponse shape (status).
	configCloseResponseTemplate = "close_response_template"

	// Webhook URL pattern (legacy, kept for compatibility with existing
	// ticketer webhook handlers in Mailroom).
	webhookBasePath = "/mr/tickets/types/" + typeGeneric + "/event_callback"
)

func init() {
	models.RegisterTicketService(typeGeneric, NewService)
}

// skipWebhookHMAC reports whether inbound webhook HMAC verification is
// disabled for this ticketer. Accepts "true", "1" or "yes" (case-insensitive).
func skipWebhookHMAC(config map[string]string) bool {
	return skipWebhookHMACValue(config[configSkipWebhookHMAC])
}

func skipWebhookHMACValue(value string) bool {
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "true" || v == "1" || v == "yes"
}

type service struct {
	rtConfig                *runtime.Config
	client                  *Client
	ticketer                *flows.Ticketer
	redactor                utils.Redactor
	projectUUID             string
	projectName             string
	openTemplate            *template.Template
	openResponseTemplate    *template.Template
	forwardTemplate         *template.Template
	forwardResponseTemplate *template.Template
	closeTemplate           *template.Template
	closeResponseTemplate   *template.Template
}

// NewService creates a new generic ticket service from the given config map.
// Required keys: base_url, api_token. webhook_secret is required unless
// skip_webhook_hmac is true.
func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, model *models.Ticketer, ctx context.Context, db models.Queryer) (models.TicketService, error) {
	config := model.ConfigMap()
	baseURL := strings.TrimSpace(config[configBaseURL])
	apiToken := strings.TrimSpace(config[configAPIToken])
	webhookSecret := strings.TrimSpace(config[configWebhookSecret])
	skipHMAC := skipWebhookHMAC(config)

	if baseURL == "" || apiToken == "" {
		return nil, errors.New("missing base_url, api_token or webhook_secret in generic ticketer config")
	}
	if !skipHMAC && webhookSecret == "" {
		return nil, errors.New("missing base_url, api_token or webhook_secret in generic ticketer config")
	}

	routes := Routes{
		OpenTicket:     config[configRouteOpen],
		ForwardMessage: config[configRouteForward],
		CloseTicket:    config[configRouteClose],
		ReopenTicket:   config[configRouteReopen],
		SendHistory:    config[configRouteHistory],
	}

	var openTmpl *template.Template
	if src := strings.TrimSpace(config[configOpenTemplate]); src != "" {
		var err error
		openTmpl, err = parseOpenTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	var openRespTmpl *template.Template
	if src := strings.TrimSpace(config[configOpenResponseTemplate]); src != "" {
		var err error
		openRespTmpl, err = parseOpenResponseTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	var forwardTmpl *template.Template
	if src := strings.TrimSpace(config[configForwardTemplate]); src != "" {
		var err error
		forwardTmpl, err = parseForwardTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	var forwardRespTmpl *template.Template
	if src := strings.TrimSpace(config[configForwardResponseTemplate]); src != "" {
		var err error
		forwardRespTmpl, err = parseForwardResponseTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	var closeTmpl *template.Template
	if src := strings.TrimSpace(config[configCloseTemplate]); src != "" {
		var err error
		closeTmpl, err = parseCloseTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	var closeRespTmpl *template.Template
	if src := strings.TrimSpace(config[configCloseResponseTemplate]); src != "" {
		var err error
		closeRespTmpl, err = parseCloseResponseTemplate(src)
		if err != nil {
			return nil, err
		}
	}

	redactArgs := []string{apiToken}
	if webhookSecret != "" {
		redactArgs = append(redactArgs, webhookSecret)
	}

	return &service{
		rtConfig:                rtCfg,
		client:                  NewClient(httpClient, httpRetries, baseURL, apiToken, WithRoutes(routes)),
		ticketer:                ticketer,
		redactor:                utils.NewRedactor(flows.RedactionMask, redactArgs...),
		projectUUID:             config[configProjectUUID],
		projectName:             config[configProjectName],
		openTemplate:            openTmpl,
		openResponseTemplate:    openRespTmpl,
		forwardTemplate:         forwardTmpl,
		forwardResponseTemplate: forwardRespTmpl,
		closeTemplate:           closeTmpl,
		closeResponseTemplate:   closeRespTmpl,
	}, nil
}

// Open opens a new ticket on the partner side, mapping Mailroom session +
// topic + assignee into the OpenRequest documented in section 2.1 of the
// spec.
func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticket := flows.OpenTicket(s.ticketer, topic, body, assignee)
	contact := session.Contact()

	contactDTO := Contact{
		UUID:     string(contact.UUID()),
		Name:     contact.Name(),
		Language: string(contact.Language()),
	}

	// Respect the org-level redaction policy: anonymize the contact name
	// when URNs would otherwise leak identity.
	if session.Environment().RedactionPolicy() == envs.RedactionPolicyURNs {
		contactDTO.Name = fmt.Sprintf("%d", contact.ID())
	}

	if preferred := contact.PreferredURN(); preferred != nil {
		contactDTO.URN = preferred.URN().String()
	} else if urnList := contact.URNs(); len(urnList) > 0 {
		contactDTO.URN = urnList[0].URN().String()
	} else {
		return nil, errors.New("contact has no URNs to open generic ticket with")
	}
	for _, u := range contact.URNs() {
		contactDTO.URNs = append(contactDTO.URNs, u.URN().String())
	}

	req := &OpenRequest{
		TicketID: string(ticket.UUID()),
		Contact:  contactDTO,
		Body:     body,
		OpenedAt: dates.Now(),
	}

	if topic != nil {
		req.Topic = &Topic{
			UUID:      string(topic.UUID()),
			Name:      topic.Name(),
			QueueUUID: string(topic.QueueUUID()),
		}
	}
	if assignee != nil {
		req.Assignee = &Assignee{
			Email: assignee.Email(),
			Name:  assignee.Name(),
		}
	}

	req.Metadata = s.buildOpenMetadata(session)

	idempotencyKey := "open-" + string(ticket.UUID())

	var payload interface{} = req
	if s.openTemplate != nil {
		templatedBody, err := renderOpenTemplate(s.openTemplate, req)
		if err != nil {
			return nil, errors.Wrap(err, "error rendering open_template")
		}
		payload = json.RawMessage(templatedBody)
	}

	trace, err := s.client.openTicketRequest(payload, idempotencyKey)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		// A 409 with an existing external_id in `details` means the partner
		// already has this ticket open from a prior attempt — treat as
		// success and reuse the existing external_id.
		var ce *ClientError
		if errors.As(err, &ce) && ce.StatusCode == http.StatusConflict && len(ce.Details) > 0 {
			existing := &struct {
				ExternalID string `json:"external_id"`
			}{}
			if jsonErr := json.Unmarshal(ce.Details, existing); jsonErr == nil && existing.ExternalID != "" {
				ticket.SetExternalID(existing.ExternalID)
				return ticket, nil
			}
		}
		return nil, errors.Wrap(err, "error opening generic ticket")
	}

	resp, err := s.parseOpenResponse(trace.ResponseBody)
	if err != nil {
		return nil, err
	}

	if resp.ExternalID == "" {
		return nil, errors.New("generic ticketer did not return an external_id")
	}
	ticket.SetExternalID(resp.ExternalID)
	return ticket, nil
}

// parseOpenResponse maps or decodes the partner Open response into OpenResponse.
func (s *service) parseOpenResponse(raw []byte) (*OpenResponse, error) {
	if s.openResponseTemplate != nil {
		resp, err := mapOpenResponse(s.openResponseTemplate, raw)
		if err != nil {
			return nil, errors.Wrap(err, "error mapping open_response_template")
		}
		return resp, nil
	}
	return decodeOpenResponse(raw)
}

// Forward delivers an inbound contact message to the partner over the
// MessageRequest endpoint documented in section 2.2 of the spec.
func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	externalID := string(ticket.ExternalID())
	if externalID == "" {
		return errors.New("cannot forward message: ticket has no external_id")
	}

	req := &MessageRequest{
		TicketID:   string(ticket.UUID()),
		ExternalID: externalID,
		Direction:  "incoming",
		Sender: Sender{
			Type: "contact",
			ID:   ticket.Config("contact-uuid"),
			Name: ticket.Config("contact-display"),
		},
		SentAt: dates.Now(),
	}

	if string(msgUUID) != "" {
		req.MessageID = string(msgUUID)
	}
	if strings.TrimSpace(text) != "" {
		req.Text = text
	}
	for _, a := range attachments {
		req.Attachments = append(req.Attachments, Attachment{
			ContentType: a.ContentType(),
			URL:         a.URL(),
		})
	}

	req.Metadata = buildForwardMetadata(metadata, msgExternalID)

	// Use the message UUID as the idempotency key when available so retries
	// of the same source message are coalesced on the partner side.
	idempotencyKey := ""
	if string(msgUUID) != "" {
		idempotencyKey = "forward-" + string(msgUUID)
	}

	var payload interface{} = req
	if s.forwardTemplate != nil {
		templatedBody, err := renderForwardTemplate(s.forwardTemplate, req)
		if err != nil {
			return errors.Wrap(err, "error rendering forward_template")
		}
		payload = json.RawMessage(templatedBody)
	}

	trace, err := s.client.forwardMessageRequest(externalID, payload, idempotencyKey)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return errors.Wrap(err, "error forwarding message to generic ticketer")
	}

	if _, err := s.parseForwardResponse(trace.ResponseBody); err != nil {
		return err
	}
	return nil
}

// parseForwardResponse maps or decodes the partner Forward response into MessageResponse.
func (s *service) parseForwardResponse(raw []byte) (*MessageResponse, error) {
	if s.forwardResponseTemplate != nil {
		resp, err := mapForwardResponse(s.forwardResponseTemplate, raw)
		if err != nil {
			return nil, errors.Wrap(err, "error mapping forward_response_template")
		}
		return resp, nil
	}
	return decodeForwardResponse(raw)
}

// Close notifies the partner that one or more tickets were closed on the
// platform side. A 409 on any single ticket is treated as already-closed and
// does not abort the loop.
func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	now := dates.Now()
	for _, t := range tickets {
		externalID := string(t.ExternalID())
		if externalID == "" {
			continue
		}
		req := &CloseRequest{
			TicketID:   string(t.UUID()),
			ExternalID: externalID,
			ClosedBy:   ActorRef{Type: "platform", ID: "system"},
			ClosedAt:   now,
		}
		idempotencyKey := fmt.Sprintf("close-%s-%d", t.UUID(), now.UnixNano())

		var payload interface{} = req
		if s.closeTemplate != nil {
			templatedBody, err := renderCloseTemplate(s.closeTemplate, req)
			if err != nil {
				return errors.Wrapf(err, "error rendering close_template for ticket %s", t.UUID())
			}
			payload = json.RawMessage(templatedBody)
		}

		trace, err := s.client.closeTicketRequest(externalID, payload, idempotencyKey)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			var ce *ClientError
			if errors.As(err, &ce) && ce.StatusCode == http.StatusConflict {
				continue
			}
			return errors.Wrapf(err, "error closing generic ticket %s", t.UUID())
		}

		if _, err := s.parseCloseResponse(trace.ResponseBody); err != nil {
			return errors.Wrapf(err, "error parsing close response for ticket %s", t.UUID())
		}
	}
	return nil
}

// parseCloseResponse maps or decodes the partner Close response into CloseResponse.
func (s *service) parseCloseResponse(raw []byte) (*CloseResponse, error) {
	if s.closeResponseTemplate != nil {
		resp, err := mapCloseResponse(s.closeResponseTemplate, raw)
		if err != nil {
			return nil, errors.Wrap(err, "error mapping close_response_template")
		}
		return resp, nil
	}
	return decodeCloseResponse(raw)
}

// Reopen notifies the partner that one or more tickets were reopened on the
// platform side.
func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	now := dates.Now()
	for _, t := range tickets {
		externalID := string(t.ExternalID())
		if externalID == "" {
			continue
		}
		req := &ReopenRequest{
			TicketID:   string(t.UUID()),
			ExternalID: externalID,
			ReopenedBy: ActorRef{Type: "platform", ID: "system"},
			ReopenedAt: now,
		}
		idempotencyKey := fmt.Sprintf("reopen-%s-%d", t.UUID(), now.UnixNano())

		trace, err := s.client.ReopenTicket(externalID, req, idempotencyKey)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrapf(err, "error reopening generic ticket %s", t.UUID())
		}
	}
	return nil
}

// SendHistory is a no-op in this stage. The optional history endpoint is part
// of the spec (section 2.5) but DB-backed population of past contact messages
// is deferred to a follow-up to keep this stage focused on the synchronous
// open / forward / close / reopen flow.
func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	return nil
}

// buildOpenMetadata gathers the optional metadata block sent on Open. All
// fields are best-effort — missing data is omitted rather than aborting the
// request.
func (s *service) buildOpenMetadata(session flows.Session) map[string]interface{} {
	metadata := map[string]interface{}{}

	if s.projectUUID != "" {
		metadata["project_uuid"] = s.projectUUID
	}
	if s.projectName != "" {
		metadata["project_name"] = s.projectName
	}

	if runs := session.Runs(); len(runs) > 0 && runs[0].Flow() != nil {
		flow := runs[0].Flow()
		metadata["flow"] = map[string]string{
			"uuid": string(flow.UUID()),
			"name": flow.Name(),
		}
	}

	if channel := session.Contact().PreferredChannel(); channel != nil {
		metadata["channel"] = map[string]string{
			"uuid":    string(channel.UUID()),
			"name":    channel.Name(),
			"address": channel.Address(),
		}
	}

	// Inform partner about the platform-side webhook base URL so they don't
	// need to hard-code it. The full per-event URL is base + suffix (see
	// section 4 of the spec).
	if s.rtConfig != nil && s.rtConfig.Domain != "" {
		metadata["webhook_base_url"] = fmt.Sprintf(
			"https://%s%s/%s",
			s.rtConfig.Domain,
			webhookBasePath,
			s.ticketer.UUID(),
		)
	}

	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

// buildForwardMetadata merges Mailroom's per-message metadata blob with the
// source message external_id (when present) into a single map for the partner.
func buildForwardMetadata(raw json.RawMessage, msgExternalID null.String) map[string]interface{} {
	out := map[string]interface{}{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if msgExternalID != null.NullString && string(msgExternalID) != "" {
		out["source_message_external_id"] = string(msgExternalID)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
