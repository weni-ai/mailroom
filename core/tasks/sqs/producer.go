package sqs

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"
)

// EnqueuePublish queues an SQS publish task to be processed asynchronously by the SQS foreman workers.
func EnqueuePublish(rt *runtime.Runtime, orgID models.OrgID, queueURL string, msg sqsclient.Message) error {
	return enqueuePublish(rt, orgID, queueURL, msg, nil, nil)
}

// EnqueuePublishWithAttributes queues an SQS publish task with message attributes. Meta is optional and only
// identifies the message in logs, it is never sent to SQS.
func EnqueuePublishWithAttributes(rt *runtime.Runtime, orgID models.OrgID, queueURL string, msg sqsclient.Message, attributes map[string]string, meta *PublishMeta) error {
	return enqueuePublish(rt, orgID, queueURL, msg, attributes, meta)
}

func enqueuePublish(rt *runtime.Runtime, orgID models.OrgID, queueURL string, msg sqsclient.Message, attributes map[string]string, meta *PublishMeta) error {
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
		Meta:        meta,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// QueuedOn is set once, here, and never changes: it orders the message in its org and measures its time in the queue
	task := &queue.Task{
		Type:     queue.SqsPublish,
		OrgID:    int(orgID),
		Task:     payloadJSON,
		QueuedOn: time.Now(),
	}
	info := newDeliveryInfo(task, payload)

	rc := rt.RP.Get()
	defer rc.Close()

	if err := enqueue(rc, task); err != nil {
		logEnqueueFailed(info, err, time.Now())
		return err
	}
	logEnqueued(info, time.Now())
	return nil
}
