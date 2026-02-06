package publishers

import (
	"encoding/json"
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

func PublishTicketCreated(rt *runtime.Runtime, orgID models.OrgID, msg TicketSQSMessage) error {
	return sqs.EnqueuePublish(rt, orgID, rt.Config.SqsTicketsQueueURL, msg)
}
