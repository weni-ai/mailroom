package rabbitmq

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/runtime/rmq"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	mailroom.AddTaskFunction(queue.RabbitmqPublish, handleRabbitmqPublish)
}

// PublishTask is the payload stored in the queue for publishing to RabbitMQ.
// Body is expected to be JSON when ContentType is application/json.
type PublishTask struct {
	Exchange    string          `json:"exchange"`
	RoutingKey  string          `json:"routing_key"`
	ContentType string          `json:"content_type"`
	Body        json.RawMessage `json:"body"`
	Attempt     int             `json:"attempt,omitempty"`
}

func handleRabbitmqPublish(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {

	if task.Type != queue.RabbitmqPublish {
		return errors.Errorf("unknown task type for rabbitmq publish: %s", task.Type)
	}

	p := &PublishTask{}
	if err := json.Unmarshal(task.Task, p); err != nil {
		return errors.Wrap(err, "error unmarshalling rabbitmq publish task")
	}

	if rt.Rabbitmq == nil {
		return errors.New("rabbitmq client not initialized")
	}

	msg := rmq.RawMessage{Body: []byte(p.Body), Type: p.ContentType}
	if err := rt.Rabbitmq.SendTo(p.Exchange, p.RoutingKey, msg); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"exchange":    p.Exchange,
			"routing_key": p.RoutingKey,
			"attempt":     p.Attempt,
			"org_id":      task.OrgID,
		}).Warn("rmq publish failed, scheduling retry")

		if p.Attempt+1 <= rt.Config.RabbitmqPublishMaxAttempts {
			// schedule async requeue after fixed delay to avoid blocking the worker
			next := *p
			next.Attempt = p.Attempt + 1
			delayMs := rt.Config.RabbitmqPublishDelayIntervalMs
			go func(payload PublishTask, orgID int, delay int) {
				time.Sleep(time.Duration(delay) * time.Millisecond)
				rc := rt.RP.Get()
				defer rc.Close()
				_ = queue.AddTask(rc, queue.RabbitmqPublish, queue.RabbitmqPublish, orgID, &payload, queue.DefaultPriority)
			}(next, task.OrgID, delayMs)

			// already scheduled retry
			return nil
		}

		// give up after max retries
		return errors.Wrap(err, "max retries reached for rabbitmq publish")
	}

	logrus.WithFields(logrus.Fields{
		"exchange":    p.Exchange,
		"routing_key": p.RoutingKey,
		"org_id":      task.OrgID,
	}).Info("rmq publish succeeded")

	return nil
}
