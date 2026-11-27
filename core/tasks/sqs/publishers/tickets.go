package publishers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/tasks/sqs"
	"github.com/nyaruka/mailroom/runtime"
)

type TicketSQSMessage struct {
	TicketUUID  uuids.UUID `json:"ticket_uuid"`
	ContactURN  urns.URN   `json:"contact_urn"`
	ProjectUUID uuids.UUID `json:"project_uuid"`
	ChannelUUID uuids.UUID `json:"channel_uuid"`
	CreatedOn   time.Time  `json:"created_on"`
}

func (m TicketSQSMessage) Marshal() ([]byte, error) { return json.Marshal(m) }
func (m TicketSQSMessage) ContentType() string      { return "application/json" }

// Where a ticket created message was produced, recorded in logs to tell the two paths apart
const (
	OriginHTTP = "http" // POST /mr/ticket/open
	OriginFlow = "flow" // ticket_opened event of a flow
)

// PublishTicketCreated queues the ticket created message for SQS. Origin says which path produced it. Enqueue
// failures are logged by the producer with the identifiers of the ticket.
func PublishTicketCreated(rt *runtime.Runtime, orgID models.OrgID, msg TicketSQSMessage, origin string) error {
	if !rt.Config.SqsPublishEnabled {
		return nil
	}
	MessageGroupId := fmt.Sprintf("%s:%s:%s", msg.ProjectUUID, msg.ChannelUUID, msg.ContactURN)
	CorrelationID := string(uuids.New())

	return sqs.EnqueuePublishWithAttributes(
		rt, orgID, rt.Config.SqsTicketsQueueURL, msg,
		map[string]string{
			"MessageGroupId": MessageGroupId,
			"CorrelationID":  CorrelationID,
		},
		&sqs.PublishMeta{
			TicketUUID:    string(msg.TicketUUID),
			CreatedOn:     msg.CreatedOn,
			Origin:        origin,
			CorrelationID: CorrelationID,
		},
	)
}
