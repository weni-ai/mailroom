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
	"github.com/sirupsen/logrus"
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
	if !rt.Config.SqsPublishEnabled {
		return nil
	}
	MessageGroupId := fmt.Sprintf("%s:%s:%s", msg.ProjectUUID, msg.ChannelUUID, msg.ContactURN)
	CorrelationID := string(uuids.New())
	enqueued := sqs.EnqueuePublishWithAttributes(
		rt, orgID, rt.Config.SqsTicketsQueueURL, msg,
		map[string]string{
			"MessageGroupId": MessageGroupId,
			"CorrelationID":  CorrelationID,
		},
	)
	if enqueued != nil {
		logrus.WithFields(logrus.Fields{
			"message_group_id": MessageGroupId,
			"correlation_id":   CorrelationID,
			"org_id":           orgID,
			"queue_url":        rt.Config.SqsTicketsQueueURL,
		}).Info("enqueued ticket created message")
		return enqueued
	}

	return nil
}
