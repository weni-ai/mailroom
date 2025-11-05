package freshchat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
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
	contactDisplay := session.Contact().Format(session.Environment())
	contactUUID := string(session.Contact().UUID())
	channelID := ""
	userID := ""

	splitName := strings.Split(contactDisplay, " ")
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
		Properties: []UserProperty{
			{Name: "external_id", Value: contactUUID},
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

	bodyMap := Conversation{} // properties and message parts will be in this map

	if !strings.HasPrefix(body, "{") {
		bodyMap.Messages = []Message{
			{
				MessagesPart: []MessagesPart{
					{
						Text: &Text{
							Content: body,
						},
					},
				},
			},
		}
	} else {
		err = jsonx.Unmarshal([]byte(body), bodyMap)
		if err != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
	}

	if bodyMap.ChannelID != "" {
		channelID = bodyMap.ChannelID
	} else {
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
		// the default channel id is the first one
		channelID = channels[0].ID
	}

	msg := &Conversation{
		ChannelID: channelID,
		Status:    "new",
		Users: []User{
			{ID: string(userID)},
		},
	}

	if len(bodyMap.Messages) > 0 && len(bodyMap.Messages[0].MessagesPart) > 0 && bodyMap.Messages[0].MessagesPart[0].Text != nil {
		msg.Messages = []Message{
			{
				MessagesPart: []MessagesPart{
					{
						Text: &Text{
							Content: bodyMap.Messages[0].MessagesPart[0].Text.Content,
						},
					},
				},
			},
		}
	} else {
		// Fallback to simple text message
		msg.Messages = []Message{
			{
				MessagesPart: []MessagesPart{
					{
						Text: &Text{
							Content: body,
						},
					},
				},
			},
		}
	}

	if bodyMap.Properties != nil {
		msg.Properties = bodyMap.Properties
	}

	resultsConversation, trace, err := s.restClient.CreateConversation(msg)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil || (len(resultsConversation.Messages) > 0 && resultsConversation.Messages[0].ErrorMessage != "") {
		if err == nil {
			err = errors.New(resultsConversation.Messages[0].ErrorMessage)
		}
		return nil, errors.Wrap(err, "error creating conversation")
	}

	ticket.SetExternalID(string(resultsConversation.ConversationID))

	return ticket, nil
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	conversationID := string(ticket.ExternalID())

	msg := &Message{
		ConversationID: conversationID,
		MessagesPart: []MessagesPart{
			{
				Text: &Text{
					Content: text,
				},
			},
		},
	}

	for _, attachment := range attachments {
		if attachment.ContentType() == "image/jpeg" || attachment.ContentType() == "image/png" || attachment.ContentType() == "image/gif" || attachment.ContentType() == "image/webp" {
			imageURL, err := s.restClient.UploadImage(attachment.URL())
			if err != nil {
				imageURL = attachment.URL()
			}
			msg.MessagesPart = append(msg.MessagesPart, MessagesPart{
				Image: &Image{
					URL: imageURL,
				},
			})
		} else if attachment.ContentType() == "video/mp4" || attachment.ContentType() == "video/quicktime" || attachment.ContentType() == "video/webm" {
			msg.MessagesPart = append(msg.MessagesPart, MessagesPart{
				Video: &Video{
					URL:         attachment.URL(),
					ContentType: attachment.ContentType(),
				},
			})
		} else {
			file, err := s.restClient.UploadFile(attachment.URL())
			if err != nil {
				file.URL = attachment.URL()
			}
			msg.MessagesPart = append(msg.MessagesPart, MessagesPart{
				File: file,
			})
		}
	}

	channelID := ticket.Config("channel_id")
	if channelID == "" {
		return errors.New("channel_id is not set")
	}

	channels, trace, _ := s.restClient.GetChannels()
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}

	if len(channels) == 0 {
		return errors.New("no freshchat channels found")
	}

	channel := channels[0]
	msg.ChannelID = channel.ID

	msg.ActorType = "user"
	msg.ActorID = ticket.Config("user_id")

	_, trace, err := s.restClient.CreateMessage(msg)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return errors.Wrap(err, "failed to create message")
	}
	return nil
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, ticket := range tickets {
		conversation := &Conversation{
			ConversationID: string(ticket.ExternalID()),
			Status:         "closed",
		}
		trace, err := s.restClient.UpdateConversation(conversation)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrap(err, "failed to close conversation")
		}
	}
	return nil
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, ticket := range tickets {
		conversation := &Conversation{
			ConversationID: string(ticket.ExternalID()),
			Status:         "open",
		}
		trace, err := s.restClient.UpdateConversation(conversation)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrap(err, "failed to reopen conversation")
		}
	}
	return nil
}

func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	return nil
}
