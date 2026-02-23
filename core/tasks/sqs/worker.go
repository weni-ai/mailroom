package sqs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	mailroom.AddTaskFunction(queue.SqsPublish, handleSqsPublish)
}

// PublishTask is the payload stored in the queue for publishing to SQS.
// Body is expected to be JSON when ContentType is application/json.
type PublishTask struct {
	QueueURL    string            `json:"queue_url"`
	ContentType string            `json:"content_type"`
	Body        json.RawMessage   `json:"body"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Attempt     int               `json:"attempt,omitempty"`
}

func handleSqsPublish(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {

	if task.Type != queue.SqsPublish {
		return errors.Errorf("unknown task type for sqs publish: %s", task.Type)
	}

	p := &PublishTask{}
	if err := json.Unmarshal(task.Task, p); err != nil {
		return errors.Wrap(err, "error unmarshalling sqs publish task")
	}

	if rt.SQS == nil {
		return errors.New("sqs client not initialized")
	}

	msg := sqsclient.RawMessage{Body: []byte(p.Body), Type: p.ContentType}

	var err error
	if len(p.Attributes) > 0 {
		err = rt.SQS.SendToWithAttributes(p.QueueURL, msg, p.Attributes)
	} else {
		err = rt.SQS.SendTo(p.QueueURL, msg)
	}

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"queue_url": p.QueueURL,
			"attempt":   p.Attempt,
			"org_id":    task.OrgID,
		}).Warn("sqs publish failed, scheduling retry")

		if p.Attempt+1 <= rt.Config.SqsPublishMaxAttempts {
			// schedule async requeue after fixed delay to avoid blocking the worker
			next := *p
			next.Attempt = p.Attempt + 1
			delayMs := rt.Config.SqsPublishDelayIntervalMs
			go func(payload PublishTask, orgID int, delay int) {
				time.Sleep(time.Duration(delay) * time.Millisecond)
				rc := rt.RP.Get()
				defer rc.Close()
				_ = queue.AddTask(rc, queue.SqsPublish, queue.SqsPublish, orgID, &payload, queue.DefaultPriority)
			}(next, task.OrgID, delayMs)

			// already scheduled retry
			return nil
		}

		// give up after max retries
		return errors.Wrap(err, "max retries reached for sqs publish")
	}

	logrus.WithFields(logrus.Fields{
		"queue_url": p.QueueURL,
		"org_id":    task.OrgID,
	}).Info("sqs publish succeeded")

	return nil
}
