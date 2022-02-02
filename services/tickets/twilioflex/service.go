package twilioflex

import (
	"fmt"
	"net/http"

	"github.com/pkg/errors"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
)

const (
	typeTwilioFlex              = "twilioflex"
	configurationAuthToken      = "auth_token"
	configurationAccountSid     = "account_sid"
	configurationChatServiceSid = "chat_service_sid"
	configurationWorkspaceSid   = "workspace_sid"
	configurationFlexFlowSid    = "flex_flow_sid"
)

func init() {
	models.RegisterTicketService(typeTwilioFlex, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	ticketer   *flows.Ticketer
	redactor   utils.Redactor
}

// newService creates a new twilio flex ticket service
func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	authToken := config[configurationAuthToken]
	accountSid := config[configurationAccountSid]
	chatServiceSid := config[configurationChatServiceSid]
	workspaceSid := config[configurationWorkspaceSid]
	flexFlowSid := config[configurationFlexFlowSid]
	if authToken != "" && accountSid != "" && chatServiceSid != "" && workspaceSid != "" {
		return &service{
			rtConfig:   rtCfg,
			ticketer:   ticketer,
			restClient: NewClient(httpClient, httpRetries, authToken, accountSid, chatServiceSid, workspaceSid, flexFlowSid),
			redactor:   utils.NewRedactor(flows.RedactionMask, authToken, accountSid, chatServiceSid, workspaceSid),
		}, nil
	}
	return nil, errors.New("missing auth_token or account_sid or chat_service_sid or workspace_sid in twilio flex config")
}

func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticket := flows.OpenTicket(s.ticketer, topic, body, assignee)
	contact := session.Contact()
	chatUser := &CreateChatUserParams{
		Identity:     fmt.Sprint(contact.ID()),
		FriendlyName: contact.Name(),
	}
	contactUser, trace, err := s.restClient.FetchUser(chatUser.Identity)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil && trace.Response.StatusCode != 404 {
		return nil, errors.Wrapf(err, "failed to get twilio chat user")
	}
	if contactUser == nil {
		_, trace, err := s.restClient.CreateUser(chatUser)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return nil, errors.Wrap(err, "failed to create twilio chat user")
		}
	}

	flexChannelParams := &CreateFlexChannelParams{
		FlexFlowSid:          s.restClient.flexFlowSid,
		Identity:             fmt.Sprint(contact.ID()),
		ChatUserFriendlyName: contact.Name(),
		ChatFriendlyName:     contact.Name(),
	}
	newFlexChannel, trace, err := s.restClient.CreateFlexChannel(flexChannelParams)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to create twilio flex chat channel")
	}

	callbackURL := fmt.Sprintf(
		"https://%s/mr/tickets/types/twilioflex/event_callback/%s/%s",
		s.rtConfig.Domain,
		s.ticketer.UUID(),
		ticket.UUID(),
	)

	channelWebhook := &CreateChatChannelWebhookParams{
		ConfigurationUrl:        callbackURL,
		ConfigurationFilters:    []string{"onMessageSent", "onChannelUpdated"},
		ConfigurationMethod:     "POST",
		ConfigurationRetryCount: 1,
		Type:                    "webhook",
	}
	_, trace, err = s.restClient.CreateFlexChannelWebhook(channelWebhook, newFlexChannel.Sid)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to create channel webhook")
	}

	ticket.SetExternalID(newFlexChannel.Sid)
	return ticket, nil
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, logHTTP flows.HTTPLogCallback) error {
	identity := fmt.Sprint(ticket.ContactID())
	msg := &ChatMessage{
		From:       identity,
		Body:       text,
		ChannelSid: string(ticket.ExternalID()),
	}
	// TODO: attachments
	_, trace, err := s.restClient.CreateMessage(msg)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return errors.Wrap(err, "error calling Twilio")
	}
	return nil
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, t := range tickets {
		flexChannel, trace, err := s.restClient.FetchFlexChannel(string(t.ExternalID()))
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrap(err, "error calling Twilio API")
		}

		_, trace, err = s.restClient.CompleteTask(flexChannel.TaskSid)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrap(err, "error calling Twilio API")
		}
	}
	return nil
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return errors.New("Twilio Flex ticket type doesn't support reopening")
}