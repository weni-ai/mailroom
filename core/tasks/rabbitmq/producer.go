package rabbitmq

import (
	"encoding/json"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/runtime/rmq"
)

// EnqueuePublish queues a RabbitMQ publish task to be processed asynchronously by the RabbitMQ foreman workers.
func EnqueuePublish(rt *runtime.Runtime, orgID models.OrgID, exchange string, routingKey string, msg rmq.Message) error {
	if msg == nil || rt == nil || rt.RP == nil {
		return nil
	}
	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	payload := &PublishTask{
		Exchange:    exchange,
		RoutingKey:  routingKey,
		ContentType: msg.ContentType(),
		Body:        json.RawMessage(body),
		Attempt:     0,
	}

	rc := rt.RP.Get()
	defer rc.Close()
	return queue.AddTask(rc, queue.RabbitmqPublish, queue.RabbitmqPublish, int(orgID), payload, queue.DefaultPriority)
}
