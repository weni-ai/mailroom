package sqs

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/mailroom/core/queue"

	"github.com/sirupsen/logrus"
)

// This file is the single place where SQS publish logs are built. Field names and values are an operational
// interface (see specs/005-fix-sqs-ticket-publish/contracts/publish-logs.md), so nothing else in the SQS publish
// path assembles log fields by hand. Never log the contact URN, the message group id, the message attributes or
// the raw task body here: the body carries the URN.

const (
	// logTimeFormat is the format of every time field: UTC with explicit milliseconds
	logTimeFormat = "2006-01-02T15:04:05.000Z"

	// sqsSlowDeliveryThreshold is how long after ticket creation a delivery is reported as slow
	sqsSlowDeliveryThreshold = 5 * time.Minute

	slowDeliveryMessage = "sqs ticket delivery exceeded threshold"

	originUnknown = "unknown"
)

// log field names
const (
	fieldComp                   = "comp"
	fieldEvent                  = "event"
	fieldResult                 = "result"
	fieldTicketUUID             = "ticket_uuid"
	fieldCorrelationID          = "correlation_id"
	fieldOrgID                  = "org_id"
	fieldQueueURL               = "queue_url"
	fieldOrigin                 = "origin"
	fieldAttempt                = "attempt"
	fieldCreatedOn              = "created_on"
	fieldQueuedOn               = "queued_on"
	fieldEnqueuedAt             = "enqueued_at"
	fieldAttemptedAt            = "attempted_at"
	fieldSentAt                 = "sent_at"
	fieldNextAttemptAt          = "next_attempt_at"
	fieldFirstAttemptAt         = "first_attempt_at"
	fieldCreatedToQueuedMs      = "created_to_queued_ms"
	fieldQueuedToSentMs         = "queued_to_sent_ms"
	fieldCreatedToSentMs        = "created_to_sent_ms"
	fieldSlowThresholdMs        = "slow_threshold_ms"
	fieldQueuedToFirstAttemptMs = "queued_to_first_attempt_ms"
	fieldFirstAttemptToSentMs   = "first_attempt_to_sent_ms"
	fieldError                  = "error"
	fieldRecoveredCount         = "recovered_count"
	fieldRecovered              = "recovered"
)

// log component and event names
const (
	logComp = "sqs_publish"

	eventEnqueued           = "enqueued"
	eventAttempt            = "attempt"
	eventSent               = "sent"
	eventSlowDelivery       = "slow_delivery"
	eventRetryScheduled     = "retry_scheduled"
	eventRetryPersistFailed = "retry_persist_failed"
	eventDeadLettered       = "dead_lettered"
	eventRecovered          = "recovered"
	eventAckFailed          = "ack_failed"
	eventReclaimDropped     = "reclaim_dropped"
)

// log result values
const (
	resultSent         = "sent"
	resultFailed       = "failed"
	resultRescheduled  = "rescheduled"
	resultDeadLettered = "dead_lettered"
)

// deliveryInfo is what identifies a message in logs. Attempt is the 1-based number of the attempt being logged.
type deliveryInfo struct {
	TicketUUID     string
	CorrelationID  string
	Origin         string
	QueueURL       string
	OrgID          int
	CreatedOn      time.Time
	QueuedOn       time.Time
	FirstAttemptAt time.Time
	Attempt        int
}

// newDeliveryInfo derives the log identity of a task. Tasks queued before `meta` existed are still valid: their
// ticket uuid and creation time come from the SQS body, the correlation id from the attributes and the origin
// is "unknown".
func newDeliveryInfo(task *queue.Task, p *PublishTask) deliveryInfo {
	info := deliveryInfo{
		TicketUUID:    "",
		CorrelationID: p.Attributes["CorrelationID"],
		Origin:        originUnknown,
		QueueURL:      p.QueueURL,
		OrgID:         task.OrgID,
		QueuedOn:      task.QueuedOn,
		Attempt:       p.Attempt + 1,
	}

	// the body is only read for the two non-sensitive fields below, never logged
	var body struct {
		TicketUUID string    `json:"ticket_uuid"`
		CreatedOn  time.Time `json:"created_on"`
	}
	if err := json.Unmarshal(p.Body, &body); err == nil {
		info.TicketUUID = body.TicketUUID
		info.CreatedOn = body.CreatedOn
	}

	if m := p.Meta; m != nil {
		if m.TicketUUID != "" {
			info.TicketUUID = m.TicketUUID
		}
		if !m.CreatedOn.IsZero() {
			info.CreatedOn = m.CreatedOn
		}
		if m.CorrelationID != "" {
			info.CorrelationID = m.CorrelationID
		}
		if m.Origin != "" {
			info.Origin = m.Origin
		}
		if m.FirstAttemptAt != nil {
			info.FirstAttemptAt = *m.FirstAttemptAt
		}
	}
	return info
}

// fields returns the identification fields present in every event
func (d deliveryInfo) fields(event string) logrus.Fields {
	return logrus.Fields{
		fieldComp:          logComp,
		fieldEvent:         event,
		fieldTicketUUID:    d.TicketUUID,
		fieldCorrelationID: d.CorrelationID,
		fieldOrgID:         d.OrgID,
		fieldQueueURL:      d.QueueURL,
		fieldOrigin:        d.Origin,
	}
}

func fmtTime(t time.Time) string {
	return t.UTC().Format(logTimeFormat)
}

func millis(d time.Duration) int64 {
	return d.Milliseconds()
}

// logEnqueued records that a message was accepted into the queue
func logEnqueued(d deliveryInfo, enqueuedAt time.Time) {
	f := d.fields(eventEnqueued)
	f[fieldCreatedOn] = fmtTime(d.CreatedOn)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	f[fieldEnqueuedAt] = fmtTime(enqueuedAt)
	logrus.WithFields(f).Info("sqs ticket message enqueued")
}

// logEnqueueFailed records that a message could not be put in the queue
func logEnqueueFailed(d deliveryInfo, err error, enqueuedAt time.Time) {
	f := d.fields(eventEnqueued)
	f[fieldResult] = resultFailed
	f[fieldCreatedOn] = fmtTime(d.CreatedOn)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	f[fieldEnqueuedAt] = fmtTime(enqueuedAt)
	f[fieldError] = err.Error()
	logrus.WithFields(f).Error("sqs ticket message enqueue failed")
}

// logAttempt records the start of a send to SQS
func logAttempt(d deliveryInfo, attemptedAt time.Time) {
	f := d.fields(eventAttempt)
	f[fieldAttempt] = d.Attempt
	f[fieldAttemptedAt] = fmtTime(attemptedAt)
	logrus.WithFields(f).Info("sqs ticket send attempt")
}

// logSent records a successful send
func logSent(d deliveryInfo, sentAt time.Time) {
	f := d.fields(eventSent)
	f[fieldResult] = resultSent
	f[fieldAttempt] = d.Attempt
	f[fieldSentAt] = fmtTime(sentAt)
	f[fieldCreatedOn] = fmtTime(d.CreatedOn)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	f[fieldCreatedToSentMs] = millis(sentAt.Sub(d.CreatedOn))
	f[fieldQueuedToSentMs] = millis(sentAt.Sub(d.QueuedOn))
	logrus.WithFields(f).Info("sqs ticket message sent")
}

// isSlowDelivery says whether a message sent at `sentAt` took longer than the threshold since ticket creation
func isSlowDelivery(d deliveryInfo, sentAt time.Time) bool {
	return !d.CreatedOn.IsZero() && sentAt.Sub(d.CreatedOn) > sqsSlowDeliveryThreshold
}

// logSlowDelivery reports a delivery that took longer than the threshold. It is logged at error level so the
// existing Sentry hook forwards it, with a fixed message so occurrences group into a single issue.
func logSlowDelivery(d deliveryInfo, sentAt time.Time) {
	f := d.fields(eventSlowDelivery)
	f[fieldAttempt] = d.Attempt
	f[fieldCreatedOn] = fmtTime(d.CreatedOn)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	f[fieldSentAt] = fmtTime(sentAt)
	f[fieldCreatedToQueuedMs] = millis(d.QueuedOn.Sub(d.CreatedOn))
	f[fieldQueuedToSentMs] = millis(sentAt.Sub(d.QueuedOn))
	f[fieldCreatedToSentMs] = millis(sentAt.Sub(d.CreatedOn))
	f[fieldSlowThresholdMs] = millis(sqsSlowDeliveryThreshold)
	if !d.FirstAttemptAt.IsZero() {
		f[fieldFirstAttemptAt] = fmtTime(d.FirstAttemptAt)
		f[fieldQueuedToFirstAttemptMs] = millis(d.FirstAttemptAt.Sub(d.QueuedOn))
		f[fieldFirstAttemptToSentMs] = millis(sentAt.Sub(d.FirstAttemptAt))
	}
	logrus.WithFields(f).Error(slowDeliveryMessage)
}

// logRetryScheduled records a failed attempt whose retry was persisted
func logRetryScheduled(d deliveryInfo, sendErr error, attemptedAt, nextAttemptAt time.Time) {
	f := d.fields(eventRetryScheduled)
	f[fieldResult] = resultRescheduled
	f[fieldAttempt] = d.Attempt
	f[fieldError] = sendErr.Error()
	f[fieldAttemptedAt] = fmtTime(attemptedAt)
	f[fieldNextAttemptAt] = fmtTime(nextAttemptAt)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	logrus.WithFields(f).Warn("sqs ticket send failed, retry scheduled")
}

// logRetryPersistFailed records that the retry could not be stored, so the message stays leased until it expires.
// nextAttemptAt is when the retry was meant to happen, on the process clock because Redis could not tell.
func logRetryPersistFailed(d deliveryInfo, persistErr error, attemptedAt, nextAttemptAt time.Time) {
	f := d.fields(eventRetryPersistFailed)
	f[fieldResult] = resultFailed
	f[fieldAttempt] = d.Attempt
	f[fieldError] = persistErr.Error()
	f[fieldAttemptedAt] = fmtTime(attemptedAt)
	f[fieldNextAttemptAt] = fmtTime(nextAttemptAt)
	f[fieldQueuedOn] = fmtTime(d.QueuedOn)
	logrus.WithFields(f).Error("sqs ticket retry could not be persisted")
}

// logDeadLettered records that a message exhausted its attempts and will not be retried. attemptedAt may be zero
// when it is unknown (recovery of a lease whose worker never reported back), in which case it is omitted.
func logDeadLettered(d deliveryInfo, lastErr string, attemptedAt time.Time) {
	f := d.fields(eventDeadLettered)
	f[fieldResult] = resultDeadLettered
	f[fieldAttempt] = d.Attempt
	f[fieldError] = lastErr
	if !attemptedAt.IsZero() {
		f[fieldAttemptedAt] = fmtTime(attemptedAt)
	}
	logrus.WithFields(f).Error("sqs ticket message gave up after exhausting attempts")
}

// logAckFailed records that a finished message could not be confirmed, so it will be recovered and may be delivered twice
func logAckFailed(d deliveryInfo, ackErr error) {
	f := d.fields(eventAckFailed)
	f[fieldAttempt] = d.Attempt
	f[fieldError] = ackErr.Error()
	logrus.WithFields(f).Error("sqs ticket message could not be confirmed")
}

// recoveredItem identifies a message put back in the queue by the reclaim pass
type recoveredItem struct {
	TicketUUID    string
	CorrelationID string
	OrgID         int
	QueueURL      string
}

// logRecovered records the messages recovered by one reclaim pass. Nothing is logged for an empty pass.
func logRecovered(items []recoveredItem) {
	if len(items) == 0 {
		return
	}
	list := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		list = append(list, map[string]interface{}{
			fieldTicketUUID:    it.TicketUUID,
			fieldCorrelationID: it.CorrelationID,
			fieldOrgID:         it.OrgID,
			fieldQueueURL:      it.QueueURL,
		})
	}
	logrus.WithFields(logrus.Fields{
		fieldComp:           logComp,
		fieldEvent:          eventRecovered,
		fieldRecoveredCount: len(items),
		fieldRecovered:      list,
	}).Warn("sqs ticket messages recovered from expired leases")
}

// logReclaimDropped records a leased entry that could not be recovered because its stored task is unusable
func logReclaimDropped(orgID int, reason error) {
	logrus.WithFields(logrus.Fields{
		fieldComp:  logComp,
		fieldEvent: eventReclaimDropped,
		fieldOrgID: orgID,
		fieldError: reason.Error(),
	}).Error("sqs ticket expired lease dropped, stored task is unusable")
}

// logMaintenanceError records a failed maintenance pass. The items stay where they were and the next pass tries again.
func logMaintenanceError(what string, err error) {
	logrus.WithFields(logrus.Fields{
		fieldComp:  logComp,
		fieldError: err.Error(),
	}).Error(what)
}
