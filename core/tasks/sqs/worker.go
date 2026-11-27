package sqs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"

	"github.com/pkg/errors"
)

func init() {
	mailroom.AddTaskFunction(queue.SqsPublish, handleSqsPublish)
}

// nowFunc is the clock used for the times that go in logs and for the slow delivery check, replaced in tests
var nowFunc = time.Now

// PublishTask is the payload stored in the queue for publishing to SQS.
// Body is expected to be JSON when ContentType is application/json.
type PublishTask struct {
	QueueURL    string            `json:"queue_url"`
	ContentType string            `json:"content_type"`
	Body        json.RawMessage   `json:"body"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Attempt     int               `json:"attempt,omitempty"`
	Meta        *PublishMeta      `json:"meta,omitempty"`
}

// PublishMeta identifies a message in logs and measures its delivery. It is internal to the Redis envelope and is
// never part of the SQS body or attributes. Tasks queued before it existed have none and are handled normally.
type PublishMeta struct {
	TicketUUID    string    `json:"ticket_uuid,omitempty"`
	CreatedOn     time.Time `json:"created_on,omitempty"`
	Origin        string    `json:"origin,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`

	// FirstAttemptAt is set by the worker when the first attempt starts and carried through retries and recovery
	FirstAttemptAt *time.Time `json:"first_attempt_at,omitempty"`
}

// handleSqsPublish sends one leased task to SQS and then settles the lease, always exactly one way:
//
//   - sent: the lease is confirmed (ack)
//   - failed with attempts left: the retry is stored in the delayed set with attempt + 1 and the same queued_on, so a
//     restart does not lose it and it keeps its place in line when it comes back
//   - failed with no attempts left: the lease is confirmed and the message is given up (logged, with its identifiers)
//
// If settling fails (Redis error) the lease is left alone: it expires and the recovery pass puts the message back.
// A worker that is too slow loses the lease to the recovery pass and its settle is then a no-op, so at worst a
// message is delivered twice. SQS delivery is at-least-once and the consumer deduplicates by ticket_uuid.
//
// SqsPublishMaxAttempts is the number of resends, so a message is sent at most SqsPublishMaxAttempts + 1 times.
func handleSqsPublish(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
	if task.Type != queue.SqsPublish {
		return errors.Errorf("unknown task type for sqs publish: %s", task.Type)
	}

	rc := rt.RP.Get()
	defer rc.Close()

	p := &PublishTask{}
	if err := json.Unmarshal(task.Task, p); err != nil {
		// resending an unreadable task would fail the same way forever, so release it instead of letting it expire
		_, _ = ack(rc, task.OrgID, task.LeaseToken)
		return errors.Wrap(err, "error unmarshalling sqs publish task")
	}

	info := newDeliveryInfo(task, p)

	// The first attempt is remembered in the envelope so a slow delivery can say where its time went. It is only known
	// when the first attempt is made by a worker that survived to write the retry: a later attempt that arrives without
	// it (a task from before this existed, or one recovered after a crash) leaves it unknown instead of guessing.
	attemptedAt := nowFunc()
	if p.Attempt == 0 && (p.Meta == nil || p.Meta.FirstAttemptAt == nil) {
		if p.Meta == nil {
			p.Meta = &PublishMeta{}
		}
		p.Meta.FirstAttemptAt = &attemptedAt
		info.FirstAttemptAt = attemptedAt
	}

	logAttempt(info, attemptedAt)

	if err := send(rt, p); err != nil {
		settleFailure(rt, rc, task, p, info, err, attemptedAt)
		return nil
	}

	settleSuccess(rc, task, info)
	return nil
}

// send publishes the task to SQS
func send(rt *runtime.Runtime, p *PublishTask) error {
	if rt.SQS == nil {
		return errors.New("sqs client not initialized")
	}

	msg := sqsclient.RawMessage{Body: []byte(p.Body), Type: p.ContentType}
	if len(p.Attributes) > 0 {
		return rt.SQS.SendToWithAttributes(p.QueueURL, msg, p.Attributes)
	}
	return rt.SQS.SendTo(p.QueueURL, msg)
}

// settleSuccess confirms the lease of a message that reached SQS
func settleSuccess(rc redis.Conn, task *queue.Task, info deliveryInfo) {
	sentAt := nowFunc()

	// the message is already in SQS whatever happens to the confirmation: if it fails the lease expires and the
	// message is sent again, which at-least-once delivery allows
	if _, err := ack(rc, task.OrgID, task.LeaseToken); err != nil {
		logAckFailed(info, err)
	}

	logSent(info, sentAt)
	if isSlowDelivery(info, sentAt) {
		logSlowDelivery(info, sentAt)
	}
}

// settleFailure stores the retry of a failed attempt, or gives the message up when no attempts are left
func settleFailure(rt *runtime.Runtime, rc redis.Conn, task *queue.Task, p *PublishTask, info deliveryInfo, sendErr error, attemptedAt time.Time) {
	if p.Attempt >= rt.Config.SqsPublishMaxAttempts {
		ok, err := ack(rc, task.OrgID, task.LeaseToken)
		if err != nil {
			logAckFailed(info, err)
			return
		}
		if ok {
			logDeadLettered(info, sendErr.Error(), attemptedAt)
		}
		return
	}

	delay := time.Duration(rt.Config.SqsPublishDelayIntervalMs) * time.Millisecond

	next := *p
	next.Attempt = p.Attempt + 1

	nextTask := *task
	nextTask.LeaseToken = ""

	var err error
	nextTask.Task, err = json.Marshal(&next)
	if err != nil {
		logRetryPersistFailed(info, err, attemptedAt, nowFunc().Add(delay))
		return
	}

	nextAttemptAt, ok, err := reschedule(rc, task.OrgID, task.LeaseToken, &nextTask, delay)
	if err != nil {
		logRetryPersistFailed(info, err, attemptedAt, nowFunc().Add(delay))
		return
	}

	// when the lease was already taken over, the recovery pass owns the message and has logged it
	if ok {
		logRetryScheduled(info, sendErr, attemptedAt, nextAttemptAt)
	}
}
