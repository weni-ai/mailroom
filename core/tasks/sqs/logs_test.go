package sqs

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/queue"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var timeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

func testInfo() deliveryInfo {
	return deliveryInfo{
		TicketUUID:     testTicketID,
		CorrelationID:  testCorrID,
		Origin:         "http",
		QueueURL:       testQueueURL,
		OrgID:          7,
		CreatedOn:      time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC),
		QueuedOn:       time.Date(2024, 10, 30, 12, 0, 1, 500_000_000, time.UTC),
		FirstAttemptAt: time.Date(2024, 10, 30, 12, 0, 2, 0, time.UTC),
		Attempt:        2,
	}
}

// Q-06 / FR-015 / FR-016: every event uses fixed field names, UTC times with milliseconds, integer durations and never
// carries the URN, the message group id or a raw body
func TestLogsContract(t *testing.T) {
	hook := captureLogs(t)

	info := testInfo()
	sentAt := info.CreatedOn.Add(10 * time.Minute)
	attemptedAt := time.Date(2024, 10, 30, 9, 0, 3, 123_000_000, time.FixedZone("BRT", -3*3600)) // not UTC on purpose
	nextAt := attemptedAt.Add(2 * time.Second)

	logEnqueued(info, attemptedAt)
	logEnqueueFailed(info, errors.New("redis down"), attemptedAt)
	logAttempt(info, attemptedAt)
	logSent(info, sentAt)
	logSlowDelivery(info, sentAt)
	logRetryScheduled(info, errors.New("boom"), attemptedAt, nextAt)
	logRetryPersistFailed(info, errors.New("redis down"), attemptedAt, nextAt)
	logDeadLettered(info, "boom", attemptedAt)
	logAckFailed(info, errors.New("redis down"))
	logRecovered([]recoveredItem{{TicketUUID: testTicketID, CorrelationID: testCorrID, OrgID: 7, QueueURL: testQueueURL}})
	logReclaimDropped(7, errors.New("unreadable"))

	required := []struct {
		event  string
		level  logrus.Level
		fields []string
	}{
		{eventEnqueued, logrus.InfoLevel, []string{fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldOrigin, fieldCreatedOn, fieldQueuedOn, fieldEnqueuedAt}},
		{eventAttempt, logrus.InfoLevel, []string{fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldAttemptedAt}},
		{eventSent, logrus.InfoLevel, []string{fieldResult, fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldSentAt, fieldCreatedOn, fieldQueuedOn, fieldCreatedToSentMs, fieldQueuedToSentMs}},
		{eventSlowDelivery, logrus.ErrorLevel, []string{fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldOrigin, fieldAttempt, fieldCreatedOn, fieldQueuedOn, fieldSentAt, fieldCreatedToQueuedMs, fieldQueuedToSentMs, fieldCreatedToSentMs, fieldSlowThresholdMs, fieldFirstAttemptAt, fieldQueuedToFirstAttemptMs, fieldFirstAttemptToSentMs}},
		{eventRetryScheduled, logrus.WarnLevel, []string{fieldResult, fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldError, fieldAttemptedAt, fieldNextAttemptAt, fieldQueuedOn}},
		{eventRetryPersistFailed, logrus.ErrorLevel, []string{fieldResult, fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldError, fieldAttemptedAt, fieldNextAttemptAt, fieldQueuedOn}},
		{eventDeadLettered, logrus.ErrorLevel, []string{fieldResult, fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldAttemptedAt, fieldError}},
		{eventAckFailed, logrus.ErrorLevel, []string{fieldTicketUUID, fieldCorrelationID, fieldOrgID, fieldQueueURL, fieldAttempt, fieldError}},
		{eventRecovered, logrus.WarnLevel, []string{fieldRecoveredCount, fieldRecovered}},
		{eventReclaimDropped, logrus.ErrorLevel, []string{fieldOrgID, fieldError}},
	}

	for _, tc := range required {
		entries := eventsOf(hook, tc.event)
		require.NotEmpty(t, entries, tc.event)

		e := entries[0] // the failed variant of enqueued is checked below
		assert.Equal(t, tc.level, e.Level, tc.event)
		assert.Equal(t, logComp, e.Data[fieldComp], tc.event)
		for _, f := range tc.fields {
			assert.Contains(t, e.Data, f, "%s must carry %s", tc.event, f)
		}
	}

	// the failed enqueue is an error with a result and the same identification
	enq := eventsOf(hook, eventEnqueued)
	require.Len(t, enq, 2)
	assert.Equal(t, logrus.ErrorLevel, enq[1].Level)
	assert.Equal(t, resultFailed, enq[1].Data[fieldResult])
	assert.Equal(t, "redis down", enq[1].Data[fieldError])
	assert.Equal(t, logrus.InfoLevel, enq[0].Level)

	for _, e := range hook.AllEntries() {
		for k, v := range e.Data {
			// every time field is UTC with milliseconds, whatever zone the instant came in
			if strings.HasSuffix(k, "_at") || k == fieldCreatedOn || k == fieldQueuedOn {
				assert.Regexp(t, timeRegex, v, "%s of %s", k, e.Data[fieldEvent])
			}
			// every duration is an integer number of milliseconds
			if strings.HasSuffix(k, "_ms") {
				assert.IsType(t, int64(0), v, "%s of %s", k, e.Data[fieldEvent])
			}
		}
	}

	// a time given in BRT is logged in UTC: 09:00:03.123 BRT is 12:00:03.123 UTC
	assert.Equal(t, "2024-10-30T12:00:03.123Z", eventsOf(hook, eventAttempt)[0].Data[fieldAttemptedAt])
	assert.Equal(t, "2024-10-30T12:00:05.123Z", eventsOf(hook, eventRetryScheduled)[0].Data[fieldNextAttemptAt])

	// durations are exact
	sent := eventsOf(hook, eventSent)[0]
	assert.Equal(t, int64(600000), sent.Data[fieldCreatedToSentMs])
	assert.Equal(t, int64(598500), sent.Data[fieldQueuedToSentMs])

	assertNoPersonalData(t, hook)
}

func TestLogRecoveredListsEachItemAndSkipsEmptyPasses(t *testing.T) {
	hook := captureLogs(t)

	logRecovered(nil)
	assert.Empty(t, eventsOf(hook, eventRecovered))

	logRecovered([]recoveredItem{
		{TicketUUID: "t1", CorrelationID: "c1", OrgID: 1, QueueURL: "q1"},
		{TicketUUID: "t2", CorrelationID: "c2", OrgID: 2, QueueURL: "q2"},
	})

	entries := eventsOf(hook, eventRecovered)
	require.Len(t, entries, 1)
	assert.Equal(t, 2, entries[0].Data[fieldRecoveredCount])

	items := entries[0].Data[fieldRecovered].([]map[string]interface{})
	require.Len(t, items, 2)
	assert.Equal(t, map[string]interface{}{fieldTicketUUID: "t2", fieldCorrelationID: "c2", fieldOrgID: 2, fieldQueueURL: "q2"}, items[1])
}

func TestLogSlowDeliveryWithoutFirstAttemptOmitsItsFields(t *testing.T) {
	hook := captureLogs(t)

	info := testInfo()
	info.FirstAttemptAt = time.Time{}
	logSlowDelivery(info, info.CreatedOn.Add(time.Hour))

	e := eventsOf(hook, eventSlowDelivery)[0]
	assert.NotContains(t, e.Data, fieldFirstAttemptAt)
	assert.NotContains(t, e.Data, fieldQueuedToFirstAttemptMs)
	assert.NotContains(t, e.Data, fieldFirstAttemptToSentMs)
	assert.Equal(t, slowDeliveryMessage, e.Message)
	assert.Equal(t, int64(300000), e.Data[fieldSlowThresholdMs])
}

func TestDeliveryInfoFallbacksForTasksWithoutMeta(t *testing.T) {
	p := ticketPayload(2, nil)
	task := &queue.Task{Type: queue.SqsPublish, OrgID: 7, QueuedOn: time.Now()}

	info := newDeliveryInfo(task, &p)
	assert.Equal(t, testTicketID, info.TicketUUID)
	assert.Equal(t, testCorrID, info.CorrelationID)
	assert.Equal(t, originUnknown, info.Origin)
	assert.Equal(t, 3, info.Attempt)
	assert.Equal(t, time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC), info.CreatedOn.UTC())
	assert.True(t, info.FirstAttemptAt.IsZero())

	// meta wins over what can be derived
	first := time.Date(2024, 10, 30, 12, 5, 0, 0, time.UTC)
	p.Meta = &PublishMeta{TicketUUID: "from-meta", CorrelationID: "corr-meta", Origin: "flow", CreatedOn: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), FirstAttemptAt: &first}
	info = newDeliveryInfo(task, &p)
	assert.Equal(t, "from-meta", info.TicketUUID)
	assert.Equal(t, "corr-meta", info.CorrelationID)
	assert.Equal(t, "flow", info.Origin)
	assert.Equal(t, 2024, info.CreatedOn.Year())
	assert.Equal(t, 1, int(info.CreatedOn.Month()))
	assert.True(t, first.Equal(info.FirstAttemptAt))
}
