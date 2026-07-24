package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/pkg/errors"
)

const (
	historyModeBatch    = "batch"
	historyModeOneByOne = "one_by_one"

	defaultHistoryBatchSize = 50
	defaultHistoryWindow    = 24 * time.Hour
)

func parseHistoryMode(value string) string {
	if strings.TrimSpace(strings.ToLower(value)) == historyModeOneByOne {
		return historyModeOneByOne
	}
	return historyModeBatch
}

func parseHistoryBatchSize(value string) int {
	if strings.TrimSpace(value) == "" {
		return defaultHistoryBatchSize
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return defaultHistoryBatchSize
	}
	return n
}

func parseHistoryAfter(historyAfter string) (time.Time, error) {
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

func resolveHistorySince(ticket *models.Ticket, runs []*models.FlowRun) time.Time {
	historyAfter, _ := jsonparser.GetString([]byte(ticket.Body()), "history_after")
	if historyAfter != "" {
		if parsed, err := parseHistoryAfter(historyAfter); err == nil {
			return parsed
		}
	}
	if len(runs) > 0 {
		return runs[0].CreatedOn().Add(-time.Second)
	}
	return time.Now().Add(-defaultHistoryWindow)
}

func attachmentsFromMsg(attachments []utils.Attachment) []Attachment {
	out := make([]Attachment, 0, len(attachments))
	for _, att := range attachments {
		out = append(out, Attachment{
			ContentType: att.ContentType(),
			URL:         att.URL(),
		})
	}
	return out
}

func historyMessageFromMsg(msg *models.Msg, contactUUID string) HistoryMessage {
	hm := HistoryMessage{
		MessageID:   string(msg.UUID()),
		Text:        msg.Text(),
		Attachments: attachmentsFromMsg(msg.Attachments()),
		SentAt:      msg.CreatedOn(),
	}
	if msg.Direction() == models.DirectionIn {
		hm.Direction = "incoming"
		hm.Sender = Sender{Type: "contact", ID: contactUUID}
	} else {
		hm.Direction = "outgoing"
		hm.Sender = Sender{Type: "bot"}
	}
	return hm
}

func messageRequestFromMsg(ticket *models.Ticket, externalID string, msg *models.Msg, contactUUID, contactName string) *MessageRequest {
	req := &MessageRequest{
		TicketID:   string(ticket.UUID()),
		ExternalID: externalID,
		MessageID:  string(msg.UUID()),
		SentAt:     msg.CreatedOn(),
	}
	if msg.Direction() == models.DirectionIn {
		req.Direction = "incoming"
		req.Sender = Sender{Type: "contact", ID: contactUUID, Name: contactName}
	} else {
		req.Direction = "outgoing"
		req.Sender = Sender{Type: "bot"}
	}
	if strings.TrimSpace(msg.Text()) != "" {
		req.Text = msg.Text()
	}
	req.Attachments = attachmentsFromMsg(msg.Attachments())
	return req
}

func chunkHistoryMessages(messages []HistoryMessage, size int) [][]HistoryMessage {
	if size <= 0 {
		size = defaultHistoryBatchSize
	}
	batches := make([][]HistoryMessage, 0)
	current := make([]HistoryMessage, 0, size)
	for _, m := range messages {
		current = append(current, m)
		if len(current) == size {
			batches = append(batches, current)
			current = make([]HistoryMessage, 0, size)
		}
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func (s *service) buildHistoryContact(ctx context.Context, ticket *models.Ticket, contactID models.ContactID, msgs []*models.Msg) (Contact, error) {
	contactUUID := ticket.Config("contact-uuid")
	contactName := ticket.Config("contact-display")

	if contactName == "" {
		if name, err := models.LookupContactName(ctx, s.db, ticket.OrgID(), contactID); err == nil {
			contactName = name
		}
	}

	urn := ""
	for _, msg := range msgs {
		if msg.URN() != "" {
			urn = msg.URN().String()
			break
		}
	}
	if urn == "" {
		return Contact{}, errors.New("cannot send history: contact has no URN on loaded messages")
	}

	return Contact{
		UUID: contactUUID,
		Name: contactName,
		URN:  urn,
	}, nil
}

func (s *service) buildHistoryMetadata() map[string]interface{} {
	metadata := map[string]interface{}{}
	if s.projectUUID != "" {
		metadata["project_uuid"] = s.projectUUID
	}
	if s.projectName != "" {
		metadata["project_name"] = s.projectName
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

type historyOneByOneTemplateContext struct {
	TicketID    string                 `json:"ticket_id"`
	ExternalID  string                 `json:"external_id"`
	Contact     Contact                `json:"contact"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	MessageID   string                 `json:"message_id"`
	Direction   string                 `json:"direction"`
	Sender      Sender                 `json:"sender"`
	Text        string                 `json:"text,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	SentAt      time.Time              `json:"sent_at"`
}

func historyOneByOneTemplateContextFrom(req *HistoryRequest, msg HistoryMessage) historyOneByOneTemplateContext {
	return historyOneByOneTemplateContext{
		TicketID:    req.TicketID,
		ExternalID:  req.ExternalID,
		Contact:     req.Contact,
		Metadata:    req.Metadata,
		MessageID:   msg.MessageID,
		Direction:   msg.Direction,
		Sender:      msg.Sender,
		Text:        msg.Text,
		Attachments: msg.Attachments,
		SentAt:      msg.SentAt,
	}
}

func decodeHistoryResponse(raw []byte) (*HistoryResponse, error) {
	resp := &HistoryResponse{}
	if len(raw) == 0 {
		return resp, nil
	}
	if err := json.Unmarshal(raw, resp); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling history response")
	}
	return resp, nil
}
