package sqs

import (
	"encoding/json"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"
)

// EnqueuePublish queues an SQS publish task to be processed asynchronously by the SQS foreman workers.
func EnqueuePublish(rt *runtime.Runtime, orgID models.OrgID, queueURL string, msg sqsclient.Message) error {
	if msg == nil || rt == nil || rt.RP == nil {
		return nil
	}
	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	payload := &PublishTask{
		QueueURL:    queueURL,
		ContentType: msg.ContentType(),
		Body:        json.RawMessage(body),
		Attempt:     0,
	}

	rc := rt.RP.Get()
	defer rc.Close()
	return queue.AddTask(rc, queue.SqsPublish, queue.SqsPublish, int(orgID), payload, queue.DefaultPriority)
}

// EnqueuePublishWithAttributes queues an SQS publish task with message attributes.
func EnqueuePublishWithAttributes(rt *runtime.Runtime, orgID models.OrgID, queueURL string, msg sqsclient.Message, attributes map[string]string) error {
	if msg == nil || rt == nil || rt.RP == nil {
		return nil
	}
	body, err := msg.Marshal()
	if err != nil {
		return err
	}

	payload := &PublishTask{
		QueueURL:    queueURL,
		ContentType: msg.ContentType(),
		Body:        json.RawMessage(body),
		Attributes:  attributes,
		Attempt:     0,
	}

	rc := rt.RP.Get()
	defer rc.Close()
	return queue.AddTask(rc, queue.SqsPublish, queue.SqsPublish, int(orgID), payload, queue.DefaultPriority)
}
