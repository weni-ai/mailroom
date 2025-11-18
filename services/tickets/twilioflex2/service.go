package twilioflex2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gabriel-vasile/mimetype"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	typeTwiliioFlex2                    = "twilioflex2"
	configurationAuthToken              = "auth_token"
	configurationAccountSid             = "account_sid"
	configurationInstanceSid            = "flex_instance_sid"
	configurationWorkspaceSid           = "flex_workspace_sid"
	configurationWorkflowSid            = "flex_workflow_sid"
	configurationConversationServiceSid = "conversation_service_sid"
)

var db *sqlx.DB
var lock = &sync.Mutex{}
var historyDelay = 6

func initDB(dbURL string) error {
	if db == nil {
		lock.Lock()
		defer lock.Unlock()
		newDB, err := sqlx.Open("postgres", dbURL)
		if err != nil {
			return errors.Wrapf(err, "unable to open database connection")
		}
		SetDB(newDB)
	}
	return nil
}

func SetDB(newDB *sqlx.DB) {
	db = newDB
}

func init() {
	models.RegisterTicketService(typeTwiliioFlex2, NewService)
}

type service struct {
	rtConfig               *runtime.Config
	restClient             *Client
	ticketer               *flows.Ticketer
	redactor               utils.Redactor
	instanceSid            string
	workspaceSid           string
	workflowSid            string
	conversationServiceSid string
}

func NewService(rtConfig *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	authToken := config[configurationAuthToken]
	accountSid := config[configurationAccountSid]
	instanceSid := config[configurationInstanceSid]
	workspaceSid := config[configurationWorkspaceSid]
	workflowSid := config[configurationWorkflowSid]
	conversationServiceSid := config[configurationConversationServiceSid]

	if authToken != "" && accountSid != "" && instanceSid != "" && workspaceSid != "" && workflowSid != "" && conversationServiceSid != "" {
		if err := initDB(rtConfig.DB); err != nil {
			return nil, err
		}
		return &service{
			rtConfig:               rtConfig,
			restClient:             NewClient(httpClient, httpRetries, authToken, accountSid),
			ticketer:               ticketer,
			redactor:               utils.NewRedactor(flows.RedactionMask, authToken),
			instanceSid:            instanceSid,
			workspaceSid:           workspaceSid,
			workflowSid:            workflowSid,
			conversationServiceSid: conversationServiceSid,
		}, nil
	}
	return nil, errors.New("missing auth_token or account_sid or flex_instance_sid or flex_workspace_sid or flex_workflow_sid or conversation_service_sid in twilio flex2 config")
}

func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticket := flows.OpenTicket(s.ticketer, topic, body, assignee)
	contact := session.Contact()

	userIdentity := fmt.Sprintf("%d_%s", contact.ID(), ticket.UUID())

	interactionWebhook, trace, err := s.restClient.CreateInteractionScopedWebhook(s.instanceSid, &CreateInteractionWebhookRequest{
		Type:          "interaction",
		WebhookEvents: []string{"onChannelStatusUpdated"},
		WebhookUrl:    fmt.Sprintf("https://%s/mr/tickets/types/twilioflex2/interaction_callback/%s/%s", s.rtConfig.Domain, s.ticketer.UUID(), ticket.UUID()),
		WebhookMethod: "POST",
	})
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to create interaction webhook")
	}

	bodyMap := map[string]any{}
	json.Unmarshal([]byte(body), &bodyMap)

	interactionRequest := &CreateInteractionRequest{
		Channel: InteractionChannelParam{
			Type:        "whatsapp",
			InitiatedBy: "api",
			Participants: []InteractionChannelParticipant{
				{
					Identity: userIdentity,
					Name:     contact.Name(),
				},
			},
		},
		Routing: InteractionRoutingParam{
			Type: "TaskRouter",
			Properties: InteractionRoutingProperties{
				WorkspaceSid:          s.workspaceSid,
				WorkflowSid:           s.workflowSid,
				TaskChannelUniqueName: "chat",
				Attributes: map[string]any{
					"channelType":       "whatsapp",
					"customerId":        userIdentity,
					"custom_attributes": body,
				},
			},
		},
		WebhookTtid: interactionWebhook.Ttid,
	}

	for key, value := range bodyMap {
		interactionRequest.Routing.Properties.Attributes[key] = value
	}

	interactionResponse, trace, err := s.restClient.CreateInteraction(interactionRequest)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		logrus.Debugf("error: %+v", err)
		return nil, errors.Wrap(err, "failed to create interaction")
	}

	logrus.Debugf("interaction: %+v", interactionResponse)
	attributes := interactionResponse.Routing.Properties.Attributes
	conversationSid, _ := jsonparser.GetString([]byte(attributes), "conversationSid")
	if conversationSid == "" {
		return nil, errors.New("conversationSid is not found in interaction routing properties")
	}
	ticket.SetExternalID(conversationSid)

	_, trace, err = s.restClient.CreateConversationScopedWebhook(conversationSid, &CreateConversationWebhookRequest{
		Target:               "webhook",
		ConfigurationUrl:     fmt.Sprintf("https://%s/mr/tickets/types/twilioflex2/conversation_callback/%s/%s", s.rtConfig.Domain, s.ticketer.UUID(), ticket.UUID()),
		ConfigurationMethod:  "POST",
		ConfigurationFilters: []string{"onMessageAdded"},
	})
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		logrus.Debugf("error: %+v", err)
		return nil, errors.Wrap(err, "failed to create conversation webhook")
	}

	logrus.Debugf("ticket: %+v", ticket)
	return ticket, nil
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	identity := fmt.Sprintf("%d_%s", ticket.ContactID(), ticket.UUID())

	if len(attachments) > 0 {
		mediaAttachments := []CreateMediaParams{}
		for _, attachment := range attachments {
			attUrl := attachment.URL()
			req, err := http.NewRequest("GET", attUrl, nil)
			if err != nil {
				return err
			}
			resp, err := httpx.DoTrace(s.restClient.httpClient, req, s.restClient.httpRetries, nil, -1)
			if err != nil {
				return err
			}

			parsedURL, err := url.Parse(attUrl)
			if err != nil {
				return err
			}
			filename := path.Base(parsedURL.Path)

			mimeType := mimetype.Detect(resp.ResponseBody)

			media := CreateMediaParams{
				FileName:    filename,
				Media:       resp.ResponseBody,
				Author:      identity,
				ContentType: mimeType.String(),
			}

			mediaAttachments = append(mediaAttachments, media)
		}

		for _, mediaParams := range mediaAttachments {
			media, trace, err := s.restClient.CreateMedia(s.conversationServiceSid, &mediaParams)
			if trace != nil {
				logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
			}
			if err != nil {
				return err
			}

			msg := &CreateConversationMessageRequest{
				Author: identity,
				Body:   text,
			}
			msg.MediaSid = media.Sid
			_, trace, err = s.restClient.SendCustomerMessage(string(ticket.ExternalID()), msg)
			if trace != nil {
				logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
			}
			if err != nil {
				return err
			}
		}
	}

	if strings.TrimSpace(text) != "" {
		_, trace, err := s.restClient.SendCustomerMessage(string(ticket.ExternalID()), &CreateConversationMessageRequest{
			Author: identity,
			Body:   text,
		})
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			return errors.Wrap(err, "failed to send customer message")
		}
	}
	return nil
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (s *service) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	userIdentity := fmt.Sprintf("%d_%s", contactID, ticket.UUID())
	defaultHistoryWindow := time.Now().Add(-time.Hour * 24)
	after, err := getHistoryAfter(ticket, contactID, runs)
	if err != nil {
		after = defaultHistoryWindow
	}

	cx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// get messages for history
	msgs, err := models.SelectContactMessages(cx, db, int(contactID), after)
	if err != nil {
		return errors.Wrap(err, "failed to get history messages")
	}

	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].CreatedOn().Before(msgs[j].CreatedOn())
	})

	for _, msg := range msgs {

		// send media attachments
		if len(msg.Attachments()) > 0 {
			mediaAttachments := []CreateMediaParams{}
			for _, attachment := range msg.Attachments() {
				attUrl := attachment.URL()
				req, err := http.NewRequest("GET", attUrl, nil)
				if err != nil {
					return err
				}
				resp, err := httpx.DoTrace(s.restClient.httpClient, req, s.restClient.httpRetries, nil, -1)
				if err != nil {
					return err
				}

				parsedURL, err := url.Parse(attUrl)
				if err != nil {
					return err
				}
				filename := path.Base(parsedURL.Path)

				mimeType := mimetype.Detect(resp.ResponseBody)

				media := CreateMediaParams{
					FileName:    filename,
					Media:       resp.ResponseBody,
					Author:      userIdentity,
					ContentType: mimeType.String(),
				}

				mediaAttachments = append(mediaAttachments, media)
			}

			for _, mediaParams := range mediaAttachments {
				media, trace, err := s.restClient.CreateMedia(s.conversationServiceSid, &mediaParams)
				if trace != nil {
					logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
				}
				if err != nil {
					return err
				}
				m := &CreateConversationMessageRequest{
					Author: userIdentity,
				}
				if msg.Direction() == "I" {
					m.Author = userIdentity
				} else {
					m.Author = "Bot"
				}
				m.MediaSid = media.Sid
				_, trace, err = s.restClient.SendCustomerMessage(string(ticket.ExternalID()), m)
				if trace != nil {
					logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
				}
				if err != nil {
					return errors.Wrap(err, "error calling Twilio conversations API to send message from history with media")
				}
			}
		}

		// send text message
		if strings.TrimSpace(msg.Text()) != "" {
			m := &CreateConversationMessageRequest{
				Author: userIdentity,
				Body:   msg.Text(),
			}
			if msg.Direction() == "I" {
				m.Author = userIdentity
			} else {
				m.Author = "Bot"
			}
			_, trace, err := s.restClient.SendCustomerMessage(string(ticket.ExternalID()), m)
			if trace != nil {
				logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
			}
			if err != nil {
				return errors.Wrap(err, "error calling Twilio conversations API to send message from history")
			}
		}
	}
	return nil
}

func getHistoryAfter(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun) (time.Time, error) {
	historyAfter, _ := jsonparser.GetString([]byte(ticket.Body()), "history_after")
	var after time.Time
	var err error
	if historyAfter != "" {
		after, err = parseTime(historyAfter)
		if err != nil {
			return time.Time{}, err
		}
		return after, nil
	} else if len(runs) > 0 {
		// get messages for history, based on first session run start time
		startMargin := -time.Second * 1
		after = runs[0].CreatedOn().Add(startMargin)
		return after, nil
	}
	return time.Time{}, fmt.Errorf("history after could not be determined")
}

func parseTime(historyAfter string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05-07:00",
	}

	for _, format := range formats {
		t, err := time.Parse(format, historyAfter)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse history_after: %q, expected formats: %v", historyAfter, formats)
}
