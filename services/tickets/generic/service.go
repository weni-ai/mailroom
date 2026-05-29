// Package generic implements a generic HTTP-based ticketer service.
//
// The contract spoken by this ticketer is documented in
// docs/generic-ticketer-service.md. Any partner that implements the documented
// HTTP endpoints can plug into the platform as a ticketer without requiring
// custom code in Mailroom.
package generic

import (
	"encoding/json"
	"net/http"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

const (
	typeGeneric = "generic"

	configBaseURL       = "base_url"
	configAPIToken      = "api_token"
	configWebhookSecret = "webhook_secret"
)

func init() {
	models.RegisterTicketService(typeGeneric, NewService)
}

type service struct {
	rtConfig *runtime.Config
	client   *Client
	ticketer *flows.Ticketer
	redactor utils.Redactor
}

// NewService creates a new generic ticket service.
func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	baseURL := config[configBaseURL]
	apiToken := config[configAPIToken]
	webhookSecret := config[configWebhookSecret]

	if baseURL == "" || apiToken == "" || webhookSecret == "" {
		return nil, errors.New("missing base_url, api_token or webhook_secret in generic ticketer config")
	}

	return &service{
		rtConfig: rtCfg,
		client:   NewClient(httpClient, httpRetries, baseURL, apiToken),
		ticketer: ticketer,
		redactor: utils.NewRedactor(flows.RedactionMask, apiToken, webhookSecret),
	}, nil
}

func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	return nil, errors.New("generic ticketer Open not implemented")
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	return errors.New("generic ticketer Forward not implemented")
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return errors.New("generic ticketer Close not implemented")
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return errors.New("generic ticketer Reopen not implemented")
}

func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	return nil
}
