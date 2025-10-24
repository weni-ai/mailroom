package freshchat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
)

const (
	typeFreshchat = "freshchat"
)

func init() {
	models.RegisterTicketService(typeFreshchat, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	ticketer   *flows.Ticketer
	redactor   utils.Redactor
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	baseURL := config["base_url"]
	authToken := config["auth_token"]
	if baseURL == "" || authToken == "" {
		return nil, errors.New("missing base_url or auth_token in freshchat config")
	}
	return &service{
		rtConfig:   rtCfg,
		restClient: NewClient(httpClient, httpRetries, baseURL, authToken),
		ticketer:   ticketer,
		redactor:   utils.NewRedactor(flows.RedactionMask, authToken),
	}, nil
}

func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticket := flows.OpenTicket(s.ticketer, topic, body, assignee)
	// contactDisplay := session.Contact().Format(session.Environment())
	// contactUUID := string(session.Contact().UUID())
	channelID := ""
	userID := ""

	splitName := strings.Split(session.Contact().Name(), " ")
	firstName := ""
	lastName := ""
	if len(splitName) > 0 {
		firstName = splitName[0]
	}
	if len(splitName) > 1 {
		lastName = splitName[len(splitName)-1]
	}

	phone := ""
	for _, urn := range session.Contact().URNs() {
		if urn.URN().Scheme() == urns.WhatsAppScheme || urn.URN().Scheme() == urns.TelegramScheme || urn.URN().Scheme() == urns.TelScheme {
			phone = urn.URN().Path()
		}
	}

	user := &User{
		FirstName: firstName,
		LastName:  lastName,
		Phone:     phone,
		Properties: []Property{
			{Name: "external_id", Value: session.Contact().UUID()},
		},
	}

	resultsUser, trace, err := s.restClient.CreateUser(user)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to create user")
	}

	userID = string(resultsUser.ID)

	channels, trace, err := s.restClient.GetChannels()
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to get channels")
	}
	if len(channels) == 0 {
		return nil, errors.New("no freshchat channels found")
	}

	channelID = channels[0].ID

	// todo: implement custom fields part
	msg := &Conversation{
		ChannelID: channelID,
		Messages: []Message{
			{
				MessagesPart: []MessagesPart{
					{
						Text: &Text{
							Content: body,
						},
					},
				},
			},
		},
		Status: "new",
		Users: []User{
			{ID: string(userID)},
		},
	}

	resultsConversation, trace, err := s.restClient.CreateConversation(msg)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil || resultsConversation.Messages[0].ErrorMessage != "" {
		if err == nil {
			err = errors.New(resultsConversation.Messages[0].ErrorMessage)
		}
		return nil, errors.Wrap(err, "error creating conversation")
	}

	return ticket, nil
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	return nil
}
