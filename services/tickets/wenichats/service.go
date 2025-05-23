package wenichats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
)

const (
	typeWenichats            = "wenichats"
	configurationProjectAuth = "project_auth"
	configurationSectorUUID  = "sector_uuid"
)

var db *sqlx.DB
var lock = &sync.Mutex{}

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
	models.RegisterTicketService(typeWenichats, NewService)
}

type service struct {
	rtConfig   *runtime.Config
	restClient *Client
	ticketer   *flows.Ticketer
	redactor   utils.Redactor
	sectorUUID string
}

func NewService(rtCfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	authToken := config[configurationProjectAuth]
	sectorUUID := config[configurationSectorUUID]
	baseURL := rtCfg.WenichatsServiceURL
	if authToken != "" && sectorUUID != "" {

		if err := initDB(rtCfg.DB); err != nil {
			return nil, err
		}

		return &service{
			rtConfig:   rtCfg,
			restClient: NewClient(httpClient, httpRetries, baseURL, authToken),
			ticketer:   ticketer,
			redactor:   utils.NewRedactor(flows.RedactionMask, authToken),
			sectorUUID: sectorUUID,
		}, nil
	}

	return nil, errors.New("missing project_auth or sector_uuid")
}

func (s *service) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticket := flows.OpenTicket(s.ticketer, topic, body, assignee)
	contact := session.Contact()

	roomData := &RoomRequest{Contact: &Contact{}, CustomFields: map[string]interface{}{}}

	if assignee != nil {
		roomData.UserEmail = assignee.Email()
	}

	var groups []Group
	for _, group := range contact.Groups().All() {
		g := Group{UUID: string(group.UUID()), Name: group.Name()}
		groups = append(groups, g)
	}

	roomData.Contact.ExternalID = string(contact.UUID())

	// check if the organization has restrictions in RedactionPolicy
	rp := session.Environment().RedactionPolicy()
	if rp == envs.RedactionPolicyURNs {
		roomData.Contact.Name = strconv.Itoa(int(contact.ID()))
		roomData.IsAnon = true
	} else {
		roomData.Contact.Name = contact.Name()
		roomData.IsAnon = false
	}

	roomData.SectorUUID = s.sectorUUID
	roomData.QueueUUID = string(topic.QueueUUID())
	roomData.TicketUUID = string(ticket.UUID())
	preferredURN := session.Contact().PreferredURN()
	if preferredURN != nil {
		roomData.Contact.URN = preferredURN.URN().String()
	} else {
		urns := session.Contact().URNs()
		if len(urns) == 0 {
			return nil, errors.New("failed to open ticket, no urn found for contact")
		}
		roomData.Contact.URN = urns[0].String()
	}

	if len(session.Runs()) > 0 && session.Runs()[0].Flow() != nil {
		roomData.FlowUUID = session.Runs()[0].Flow().UUID()
	}

	roomData.Contact.Groups = groups

	// if body is not configured with custom fields properly, send all fields from contact
	extra := &struct {
		CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	}{}
	err := jsonx.Unmarshal([]byte(body), extra)
	if err == nil && len(extra.CustomFields) > 0 {
		roomData.CustomFields = extra.CustomFields
	} else {
		for k, v := range contact.Fields() {
			if v != nil {
				roomData.CustomFields[k] = v.Text.Render()
			}
		}
	}

	callbackURL := fmt.Sprintf(
		"https://%s/mr/tickets/types/wenichats/event_callback/%s/%s",
		s.rtConfig.Domain,
		s.ticketer.UUID(),
		ticket.UUID(),
	)
	roomData.CallbackURL = callbackURL

	cx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticketer, err := models.LookupTicketerByUUID(cx, db, s.ticketer.UUID())

	if err != nil {
		logrus.Error(errors.Wrap(err, fmt.Sprintf("failed to lookup ticketer: %s", s.ticketer.UUID())))
		return nil, errors.Wrap(err, "failed to lookup ticketer")
	}

	if ticketer != nil && ticketer.Config("project_uuid") != "" && ticketer.Config("project_name_origin") != "" {
		roomData.ProjectInfo = &ProjectInfo{
			ProjectUUID: ticketer.Config("project_uuid"),
			ProjectName: ticketer.Config("project_name_origin"),
		}
	}

	newRoom, trace, err := s.restClient.CreateRoom(roomData)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		logrus.Error(errors.Wrap(err, fmt.Sprintf("failed to create wenichats room for: %+v", roomData)))
		return nil, errors.Wrap(err, "failed to create wenichats room")
	}

	ticket.SetExternalID(newRoom.UUID)
	return ticket, nil
}

func parseMsgAttachments(atts []utils.Attachment) []Attachment {
	msgAtts := []Attachment{}
	for _, att := range atts {
		newAtt := Attachment{
			ContentType: att.ContentType(),
			URL:         att.URL(),
		}
		msgAtts = append(msgAtts, newAtt)
	}
	return msgAtts
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	roomUUID := string(ticket.ExternalID())

	msg := &MessageRequest{
		Room:        roomUUID,
		Attachments: []Attachment{},
		Direction:   "incoming",
		CreatedOn:   dates.Now(),
		Metadata:    metadata,
	}

	if msgExternalID != null.NullString {
		msg.ExternalID = msgExternalID
	}

	if len(attachments) != 0 {
		for _, attachment := range attachments {
			msg.Attachments = append(msg.Attachments, Attachment{ContentType: attachment.ContentType(), URL: attachment.URL()})
		}
	}

	if strings.TrimSpace(text) != "" {
		msg.Text = text
	}

	_, trace, err := s.restClient.CreateMessage(msg)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return errors.Wrap(err, "error send message to wenichats")
	}

	return nil
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, t := range tickets {
		_, trace, _ := s.restClient.CloseRoom(string(t.ExternalID()))
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
	}
	return nil
}

func (s *service) Reopen(ticket []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return errors.New("wenichats ticket type doesn't support reopening")
}

func parseTime(historyAfter string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
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

func historyMsgFromMsg(msg *models.Msg) HistoryMessage {
	var direction string
	if msg.Direction() == "I" {
		direction = "incoming"
	} else {
		direction = "outgoing"
	}
	m := HistoryMessage{
		Text:        msg.Text(),
		CreatedOn:   msg.CreatedOn(),
		Attachments: parseMsgAttachments(msg.Attachments()),
		Direction:   direction,
	}
	return m
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
	}

	cx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgs, selectErr := models.SelectContactMessages(cx, db, int(contactID), after)
	if selectErr != nil {
		return errors.Wrap(selectErr, "failed to get history messages")
	}

	batchSize := 50
	batches := make([][]HistoryMessage, 0)

	currentBatch := make([]HistoryMessage, 0)

	for i := 0; i < len(msgs); i++ {
		m := historyMsgFromMsg(msgs[i])
		currentBatch = append(currentBatch, m)

		if len(currentBatch) == batchSize {
			batches = append(batches, currentBatch)
			currentBatch = make([]HistoryMessage, 0)
		}
	}

	// add the last batch if it's not empty
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	roomUUID := string(ticket.ExternalID())
	for _, batch := range batches {
		trace, err := s.restClient.SendHistoryBatch(roomUUID, batch)
		if trace != nil {
			logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
		}
		if err != nil {
			logrus.Error(errors.Wrap(err, "failed to send history batch"))
		}
	}

	logrus.Debug("History sent")
	return nil
}
