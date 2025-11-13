package freshchat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	typeFreshchat = "freshchat"
)

var db *sqlx.DB
var dbLock = &sync.Mutex{}

func initDB(dbURL string) error {
	if db == nil {
		dbLock.Lock()
		defer dbLock.Unlock()
		if db == nil {
			newDB, err := sqlx.Open("postgres", dbURL)
			if err != nil {
				return errors.Wrapf(err, "unable to open database connection")
			}
			db = newDB
		}
	}
	return nil
}

// SetDB sets the database connection (used for testing)
func SetDB(newDB *sqlx.DB) {
	db = newDB
}

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
	baseURL := config["freshchat_domain"]
	authToken := config["oauth_token"]
	if baseURL == "" || authToken == "" {
		return nil, errors.New("missing freshchat_domain or oauth_token in freshchat config")
	}
	if err := initDB(rtCfg.DB); err != nil {
		return nil, err
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

	firstName, lastName := getNames(contactDisplay)

	userID, trace := s.tryGetOrCreateUser(contactUUID, firstName, lastName, session, logHTTP)
	if userID == "" {
		return nil, errors.New("failed to get or create user for ticket")
	}

	bodyMap, _ := parseBodyMap(body, logHTTP, trace, s.redactor)
	channelID, _, err := s.resolveChannelID(bodyMap.ChannelID, s.restClient, logHTTP, s.redactor)
	if err != nil {
		return nil, err
	}

	msg := buildConversation(userID, channelID, bodyMap.Message, bodyMap.CustomFields)

	resultsConversation, createTrace, err := s.restClient.CreateConversation(msg)
	if createTrace != nil {
		logHTTP(flows.NewHTTPLog(createTrace, flows.HTTPStatusFromCode, s.redactor))
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

// Helper: parse name into first/last
func getNames(contactDisplay string) (string, string) {
	splitName := strings.Split(contactDisplay, " ")
	firstName, lastName := "", ""
	if len(splitName) > 0 {
		firstName = splitName[0]
	}
	if len(splitName) > 1 {
		lastName = splitName[len(splitName)-1]
	}
	return firstName, lastName
}

func (s *service) tryGetOrCreateUser(contactUUID, firstName, lastName string, session flows.Session, logHTTP flows.HTTPLogCallback) (string, *httpx.Trace) {
	resultsUser, trace, err := s.restClient.GetUser(contactUUID)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if resultsUser != nil {
		return resultsUser.ID, trace
	}

	var phone, email string
	for _, urn := range session.Contact().URNs() {
		if urn.URN().Scheme() == urns.WhatsAppScheme || urn.URN().Scheme() == urns.TelScheme {
			phone = urn.URN().Path()
		}
	}
	user := &User{
		FirstName:   firstName,
		LastName:    lastName,
		Phone:       phone,
		Email:       email,
		ReferenceID: contactUUID,
	}
	resultsUser, createTrace, err := s.restClient.CreateUser(user)
	if createTrace != nil {
		logHTTP(flows.NewHTTPLog(createTrace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return "", createTrace
	}
	return string(resultsUser.ID), createTrace
}

type bodyFields struct {
	Message      string                 `json:"message,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	ChannelID    string                 `json:"channel_id,omitempty"`
}

func parseBodyMap(body string, logHTTP flows.HTTPLogCallback, trace *httpx.Trace, redactor utils.Redactor) (*bodyFields, *httpx.Trace) {
	bodyMap := &bodyFields{}
	if !strings.HasPrefix(body, "{") {
		bodyMap.Message = body
	} else {
		err := jsonx.Unmarshal([]byte(body), bodyMap)
		if err != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, redactor))
		}
	}
	return bodyMap, trace
}

func (s *service) resolveChannelID(bodyMapChannelID string, client *Client, logHTTP flows.HTTPLogCallback, redactor utils.Redactor) (string, *httpx.Trace, error) {
	if bodyMapChannelID != "" {
		return bodyMapChannelID, nil, nil
	}
	channels, trace, err := client.GetChannels()
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, redactor))
	}
	if err != nil {
		return "", trace, errors.Wrap(err, "failed to get channels")
	}
	if len(channels) == 0 {
		return "", trace, errors.New("no freshchat channels found")
	}
	return channels[0].ID, trace, nil
}

func buildConversation(userID, channelID, message string, customFields map[string]interface{}) *Conversation {
	text := message
	if text == "" {
		text = "New ticket"
	}
	msg := &Conversation{
		ChannelID: channelID,
		Status:    "new",
		Users: []User{
			{ID: userID},
		},
		Messages: []Message{
			{
				MessageParts: []MessageParts{
					{
						Text: &Text{
							Content: text,
						},
					},
				},
				ActorType:   "user",
				ActorID:     userID,
				UserID:      userID,
				CreatedTime: time.Now().Format(time.RFC3339),
			},
		},
		Properties: Properties{
			Value: []map[string]interface{}{},
		},
	}
	if len(customFields) > 0 {
		msg.Properties = Properties{
			Value: []map[string]interface{}{customFields},
		}
	}
	return msg
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	conversationID := string(ticket.ExternalID())
	contactUUID := ticket.Config("contact-uuid")

	msg := &Message{
		ConversationID: conversationID,
	}

	if text != "" {
		msg.MessageParts = []MessageParts{
			{
				Text: &Text{
					Content: text,
				},
			},
		}
	}

	for _, attachment := range attachments {
		if attachment.ContentType() == "image/jpeg" || attachment.ContentType() == "image/png" || attachment.ContentType() == "image/gif" || attachment.ContentType() == "image/webp" {
			imageURL, err := s.restClient.UploadImage(attachment.URL())
			if err != nil {
				imageURL = attachment.URL()
			}
			msg.MessageParts = append(msg.MessageParts, MessageParts{
				Image: &Image{
					URL: imageURL,
				},
			})
		} else if attachment.ContentType() == "video/mp4" || attachment.ContentType() == "video/quicktime" || attachment.ContentType() == "video/webm" {
			msg.MessageParts = append(msg.MessageParts, MessageParts{
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
			msg.MessageParts = append(msg.MessageParts, MessageParts{
				File: file,
			})
		}
	}

	// Get user from Freshchat using reference_id (contact UUID)
	user, trace, err := s.restClient.GetUser(contactUUID)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return errors.Wrap(err, "failed to get user from Freshchat")
	}
	if user == nil {
		return errors.New("user not found in Freshchat")
	}

	msg.ActorType = "user"
	msg.ActorID = user.ID

	_, trace, err = s.restClient.CreateMessage(msg)
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
			Status:         "resolved",
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

func parseTime(historyAfter string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05-07:00",
		time.RFC3339,
	}

	for _, format := range formats {
		t, err := time.Parse(format, historyAfter)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, errors.Errorf("unable to parse time '%s' with formats: %v", historyAfter, formats)
}

func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	historyAfter, _ := jsonparser.GetString([]byte(ticket.Body()), "history_after")
	var after time.Time
	var err error
	if historyAfter != "" {
		after, err = parseTime(historyAfter)
		if err != nil {
			return err
		}
	} else if len(runs) > 0 {
		// get messages for history, based on first session run start time
		startMargin := -time.Second * 1
		after = runs[0].CreatedOn().Add(startMargin)
	} else {
		// No history_after and no runs, nothing to send
		return nil
	}

	cx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgs, selectErr := models.SelectContactMessages(cx, db, int(contactID), after)
	if selectErr != nil {
		return errors.Wrap(selectErr, "failed to get history messages")
	}

	if len(msgs) == 0 {
		return nil
	}

	// Get contact UUID from ticket config
	contactUUID := ticket.Config("contact-uuid")
	if contactUUID == "" {
		return errors.New("contact-uuid not found in ticket config")
	}

	// Get Freshchat user ID
	user, trace, err := s.restClient.GetUser(contactUUID)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil || user == nil {
		return errors.Wrap(err, "failed to get Freshchat user for contact")
	}

	conversationID := string(ticket.ExternalID())

	// Send each message individually to preserve order and timestamps
	for _, msg := range msgs {
		freshchatMsg := &Message{
			ConversationID: conversationID,
			CreatedTime:    msg.CreatedOn().Format(time.RFC3339),
		}

		// Set actor based on message direction
		if msg.Direction() == "I" {
			// Incoming message from user
			freshchatMsg.ActorType = "user"
			freshchatMsg.ActorID = user.ID
			freshchatMsg.UserID = user.ID
		} else {
			// Outgoing message from agent/bot
			freshchatMsg.ActorType = "agent"
			// ActorID can be empty for bot messages
		}

		// Add text content
		text := msg.Text()
		if text != "" {
			freshchatMsg.MessageParts = []MessageParts{
				{
					Text: &Text{
						Content: text,
					},
				},
			}
		}

		// Add attachments
		for _, attachment := range msg.Attachments() {
			contentType := attachment.ContentType()
			switch contentType {
			case "image/jpeg", "image/png", "image/gif", "image/webp":
				imageURL, err := s.restClient.UploadImage(attachment.URL())
				if err != nil {
					imageURL = attachment.URL()
				}
				freshchatMsg.MessageParts = append(freshchatMsg.MessageParts, MessageParts{
					Image: &Image{
						URL: imageURL,
					},
				})
			case "video/mp4", "video/quicktime", "video/webm":
				freshchatMsg.MessageParts = append(freshchatMsg.MessageParts, MessageParts{
					Video: &Video{
						URL:         attachment.URL(),
						ContentType: contentType,
					},
				})
			default:
				file, err := s.restClient.UploadFile(attachment.URL())
				if err != nil {
					file = &File{
						URL:         attachment.URL(),
						ContentType: contentType,
					}
				}
				freshchatMsg.MessageParts = append(freshchatMsg.MessageParts, MessageParts{
					File: file,
				})
			}
		}

		// Skip if no content
		if len(freshchatMsg.MessageParts) == 0 {
			continue
		}

		_, trace, err = s.restClient.CreateMessage(freshchatMsg)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			logrus.Error(errors.Wrap(err, "failed to send history message"))
			return errors.Wrap(err, "failed to send history message")
		}
	}

	return nil
}
