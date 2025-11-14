package publishers

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/tasks/rabbitmq"
	"github.com/nyaruka/mailroom/runtime"
)

// TicketMessage is the publishable message for ticket creation events.
// created_on reflects the Ticket.opened_on field.
type TicketRMQMessage struct {
	UUID         flows.TicketUUID  `json:"uuid"`
	ContactUUID  flows.ContactUUID `json:"contact_uuid"`
	ProjectUUID  uuids.UUID        `json:"project_uuid"`
	TicketerType string            `json:"type"`
	CreatedOn    time.Time         `json:"created_on"`
}

func (m TicketRMQMessage) Marshal() ([]byte, error) { return json.Marshal(m) }
func (m TicketRMQMessage) ContentType() string      { return "application/json" }

// PublishTicketCreated enqueues a ticket-created message to RabbitMQ using configured exchange/key.
func PublishTicketCreated(rt *runtime.Runtime, orgID models.OrgID, msg TicketRMQMessage) error {
	return rabbitmq.EnqueuePublish(rt, orgID, rt.Config.RabbitmqTicketsExchange, rt.Config.RabbitmqTicketsRoutingKey, msg)
}
