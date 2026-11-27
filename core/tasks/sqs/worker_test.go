package sqs

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"errors"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSqsPublish_WrongType(t *testing.T) {
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()

	task := &queue.Task{Type: "other", OrgID: 1, Task: json.RawMessage(`{}`)}
	err := handleSqsPublish(ctx, rt, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type for sqs publish")
}

func TestHandleSqsPublish_UnmarshalError(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)

	// an unreadable task can never be sent: it is reported and its lease is released instead of looping until it expires
	rc := rt.RP.Get()
	defer rc.Close()
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 1, Task: json.RawMessage(`"not-an-object"`), QueuedOn: time.Now()}))
	task, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, task)

	err = handleSqsPublish(context.Background(), rt, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling sqs publish task")

	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
}

func TestHandleSqsPublish_NoClientRetries(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)
	hook := captureLogs(t)
	rt.SQS = nil

	task := leaseTask(t, rt, 1, time.Now(), ticketPayload(0, nil))
	handle(t, rt, task)

	// a missing client is a failed attempt like any other: retried, then given up
	rc := rt.RP.Get()
	defer rc.Close()
	assert.Equal(t, 1, zcard(t, rc, keyDelayed))

	retries := eventsOf(hook, eventRetryScheduled)
	require.Len(t, retries, 1)
	assert.Equal(t, "sqs client not initialized", retries[0].Data[fieldError])
}

func TestHandleSqsPublish_Success(t *testing.T) {
	failing := &sqsFailing{}
	rt, server := newTestRuntime(t, failing.handler)
	hook := captureLogs(t)

	task := leaseTask(t, rt, 12, time.Now().Add(-2*time.Second), ticketPayload(0, nil))
	handle(t, rt, task)

	// one request with the body and attributes of the task
	reqs := server.Requests()
	require.Len(t, reqs, 1)
	assert.JSONEq(t, testTicketMsg, reqs[0].MessageBody)
	assert.Equal(t, "application/json", reqs[0].MessageAttributes["ContentType"])
	assert.Equal(t, testCorrID, reqs[0].MessageAttributes["CorrelationID"])

	// lease released, org slot given back
	rc := rt.RP.Get()
	defer rc.Close()
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.Equal(t, float64(0), activeScore(t, rc, 12))

	attempts := eventsOf(hook, eventAttempt)
	require.Len(t, attempts, 1)
	assert.Equal(t, 1, attempts[0].Data[fieldAttempt])

	sent := eventsOf(hook, eventSent)
	require.Len(t, sent, 1)
	assert.Equal(t, resultSent, sent[0].Data[fieldResult])
	assert.Equal(t, testTicketID, sent[0].Data[fieldTicketUUID])
	assert.Equal(t, testCorrID, sent[0].Data[fieldCorrelationID])
	assert.Equal(t, 12, sent[0].Data[fieldOrgID])
	assert.Contains(t, sent[0].Data, fieldSentAt)
	assert.Contains(t, sent[0].Data, fieldCreatedToSentMs)
	assert.GreaterOrEqual(t, sent[0].Data[fieldQueuedToSentMs], int64(2000))
}

// Q-04: a failure with attempts left leaves the lease and waits in delayed, then keeps its place in line
func TestHandleSqsPublish_FailureIsRescheduledInPlace(t *testing.T) {
	failing := &sqsFailing{}
	failing.set(true)
	rt, _ := newTestRuntime(t, failing.handler)
	hook := captureLogs(t)

	queuedOn := time.Now().Add(-time.Minute)
	task := leaseTask(t, rt, 8, queuedOn, ticketPayload(0, nil))
	handle(t, rt, task)

	rc := rt.RP.Get()
	defer rc.Close()

	// out of the lease and into delayed, with attempt 1 and the original queued_on
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.Equal(t, float64(0), activeScore(t, rc, 8))
	require.Equal(t, 1, zcard(t, rc, keyDelayed))

	members, err := redis.ByteSlices(rc.Do("zrange", keyDelayed, 0, -1))
	require.NoError(t, err)
	delayed := &queue.Task{}
	require.NoError(t, json.Unmarshal(members[0], delayed))
	assert.True(t, queuedOn.Equal(delayed.QueuedOn))

	p := &PublishTask{}
	require.NoError(t, json.Unmarshal(delayed.Task, p))
	assert.Equal(t, 1, p.Attempt)
	require.NotNil(t, p.Meta)
	assert.NotNil(t, p.Meta.FirstAttemptAt, "first attempt time is carried through the retry")

	retries := eventsOf(hook, eventRetryScheduled)
	require.Len(t, retries, 1)
	assert.Equal(t, logrus.WarnLevel, retries[0].Level)
	assert.Equal(t, resultRescheduled, retries[0].Data[fieldResult])
	assert.Equal(t, 1, retries[0].Data[fieldAttempt])
	assert.Contains(t, retries[0].Data, fieldAttemptedAt)
	assert.Contains(t, retries[0].Data, fieldNextAttemptAt)
	assert.Contains(t, retries[0].Data, fieldError)

	// SQS recovers: the retry is promoted and goes out as attempt 2
	failing.set(false)
	n, err := promoteDue(rc, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	retry, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, retry)
	handle(t, rt, retry)

	sent := eventsOf(hook, eventSent)
	require.Len(t, sent, 1)
	assert.Equal(t, 2, sent[0].Data[fieldAttempt])
	assert.Equal(t, 0, zcard(t, rc, keyDelayed))
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
}

// Q-05: if the retry cannot be stored the task is not confirmed, so it is recovered when its lease expires
func TestHandleSqsPublish_RetryPersistFailureKeepsLease(t *testing.T) {
	failing := &sqsFailing{}
	failing.set(true)
	rt, _ := newTestRuntime(t, failing.handler)
	hook := captureLogs(t)

	rc := rt.RP.Get()
	defer rc.Close()

	// a string where the delayed set should be makes the reschedule script fail
	_, err := rc.Do("set", keyDelayed, "broken")
	require.NoError(t, err)

	task := leaseTask(t, rt, 8, time.Now(), ticketPayload(0, nil))
	handle(t, rt, task)

	// still leased, nothing moved
	assert.Equal(t, 1, zcard(t, rc, keyInflight))
	assert.Equal(t, 1, hlen(t, rc, keyInflightValue))
	assert.Equal(t, float64(1), activeScore(t, rc, 8))
	assert.Empty(t, eventsOf(hook, eventRetryScheduled))

	failed := eventsOf(hook, eventRetryPersistFailed)
	require.Len(t, failed, 1)
	assert.Equal(t, logrus.ErrorLevel, failed[0].Level)
	assert.Equal(t, resultFailed, failed[0].Data[fieldResult])
	assert.Equal(t, testTicketID, failed[0].Data[fieldTicketUUID])
	assert.Equal(t, testCorrID, failed[0].Data[fieldCorrelationID])
	assert.Equal(t, 8, failed[0].Data[fieldOrgID])
	assert.Contains(t, failed[0].Data, fieldError)
	assert.Contains(t, failed[0].Data, fieldNextAttemptAt)
	assert.Contains(t, failed[0].Data, fieldAttemptedAt)

	// once the lease expires the recovery pass puts the message back, with one more attempt
	_, err = rc.Do("zadd", keyInflight, 1, task.LeaseToken)
	require.NoError(t, err)

	recovered, err := reclaimExpired(rc, rt.Config.SqsPublishMaxAttempts, 10)
	require.NoError(t, err)
	require.Len(t, recovered, 1)
	assert.Equal(t, 1, zcard(t, rc, orgKey(8)))
}

// Q-07: with SqsPublishMaxAttempts = 1 the message goes out twice and is then given up, leaving nothing behind
func TestHandleSqsPublish_GivesUpAfterMaxAttempts(t *testing.T) {
	failing := &sqsFailing{}
	failing.set(true)
	rt, _ := newTestRuntime(t, failing.handler)
	rt.Config.SqsPublishMaxAttempts = 1
	hook := captureLogs(t)

	rc := rt.RP.Get()
	defer rc.Close()

	// first send fails and is rescheduled
	handle(t, rt, leaseTask(t, rt, 3, time.Now(), ticketPayload(0, nil)))
	require.Equal(t, 1, zcard(t, rc, keyDelayed))

	n, err := promoteDue(rc, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// second send fails and is given up
	retry, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, retry)
	handle(t, rt, retry)

	assert.Len(t, eventsOf(hook, eventAttempt), 2)
	assert.Len(t, eventsOf(hook, eventRetryScheduled), 1)

	dead := eventsOf(hook, eventDeadLettered)
	require.Len(t, dead, 1)
	assert.Equal(t, logrus.ErrorLevel, dead[0].Level)
	assert.Equal(t, resultDeadLettered, dead[0].Data[fieldResult])
	assert.Equal(t, testTicketID, dead[0].Data[fieldTicketUUID])
	assert.Equal(t, testCorrID, dead[0].Data[fieldCorrelationID])
	assert.Equal(t, 3, dead[0].Data[fieldOrgID])
	assert.Equal(t, testQueueURL, dead[0].Data[fieldQueueURL])
	assert.Equal(t, 2, dead[0].Data[fieldAttempt])
	assert.Contains(t, dead[0].Data, fieldAttemptedAt)
	assert.Contains(t, dead[0].Data, fieldError)

	// no retry left, no lease left, active never negative and no extra structure was created
	assert.Equal(t, 0, zcard(t, rc, keyDelayed))
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.GreaterOrEqual(t, activeScore(t, rc, 3), float64(0))
	assert.Subset(t, []string{keyActive}, redisKeys(t, rt))
}

// Q-11: a task queued by a version without `meta` is sent normally and identified from its body and attributes
func TestHandleSqsPublish_TaskWithoutMeta(t *testing.T) {
	failing := &sqsFailing{}
	rt, server := newTestRuntime(t, failing.handler)
	hook := captureLogs(t)

	task := leaseTask(t, rt, 2, time.Now(), ticketPayload(0, nil))
	handle(t, rt, task)

	assert.Len(t, server.Requests(), 1)

	sent := eventsOf(hook, eventSent)
	require.Len(t, sent, 1)
	assert.Equal(t, originUnknown, sent[0].Data[fieldOrigin])
	assert.Equal(t, testTicketID, sent[0].Data[fieldTicketUUID])
	assert.Equal(t, testCorrID, sent[0].Data[fieldCorrelationID])
	assert.Equal(t, "2024-10-30T12:00:00.000Z", sent[0].Data[fieldCreatedOn])
}

// Q-09: with two workers, a send that hangs for seconds does not hold back a healthy message of another org
func TestForeman_SlowSendDoesNotBlockOtherSends(t *testing.T) {
	rt, server := newTestRuntime(t, func(n int, req testsuite.SQSRequest) testsuite.SQSResponse {
		if strings.Contains(req.MessageBody, "slow-ticket") {
			return testsuite.SQSResponse{Delay: 3 * time.Second}
		}
		return testsuite.SQSResponse{}
	})
	hook := captureLogs(t)
	_ = server

	rc := rt.RP.Get()
	defer rc.Close()

	slow := ticketPayload(0, nil)
	slow.Body = json.RawMessage(`{"ticket_uuid":"slow-ticket","created_on":"2024-10-30T12:00:00Z"}`)
	healthy := ticketPayload(0, nil)

	// the slow message is first in line, so it is the one a worker holds while the other is served
	for i, tc := range []struct {
		org int
		p   PublishTask
	}{{1, slow}, {2, healthy}} {
		data, err := json.Marshal(&tc.p)
		require.NoError(t, err)
		require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: tc.org, Task: data, QueuedOn: time.Now().Add(time.Duration(i) * time.Millisecond)}))
	}

	wg := &sync.WaitGroup{}
	foreman := mailroom.NewForeman(rt, wg, queue.SqsPublish, 2)
	foreman.Start()
	defer foreman.Stop()

	sentFor := func(org int) bool {
		for _, e := range eventsOf(hook, eventSent) {
			if e.Data[fieldOrgID] == org {
				return true
			}
		}
		return false
	}

	start := time.Now()
	require.Eventually(t, func() bool { return sentFor(2) }, time.Second, 20*time.Millisecond, "the healthy message must not wait for the slow one")
	assert.Less(t, time.Since(start), time.Second)
	assert.False(t, sentFor(1), "the slow send is still in flight")

	// the slow one completes on its own and everything is settled
	require.Eventually(t, func() bool { return sentFor(1) }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.GreaterOrEqual(t, activeScore(t, rc, 1), float64(0))
}

// Q-13: only a delivery that took strictly more than 5 minutes since ticket creation is reported, once, with where the time went
func TestHandleSqsPublish_SlowDelivery(t *testing.T) {
	created := time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC)
	queuedOn := created.Add(2 * time.Second)
	firstAttempt := created.Add(10 * time.Second)

	tcs := []struct {
		label     string
		sentAfter time.Duration
		attempt   int
		meta      *PublishMeta
		slow      bool
	}{
		{"4m59s is not slow", 4*time.Minute + 59*time.Second, 0, nil, false},
		{"exactly 5m is not slow", 5 * time.Minute, 0, nil, false},
		{"5m1s on the third attempt is slow", 5*time.Minute + time.Second, 2, &PublishMeta{CreatedOn: created, Origin: "flow", FirstAttemptAt: &firstAttempt}, true},
		{"5m1s without a known first attempt is still reported", 5*time.Minute + time.Second, 1, nil, true},
	}

	for _, tc := range tcs {
		failing := &sqsFailing{}
		rt, _ := newTestRuntime(t, failing.handler)
		hook := captureLogs(t)

		sentAt := created.Add(tc.sentAfter)
		nowFunc = func() time.Time { return sentAt }

		handle(t, rt, leaseTask(t, rt, 4, queuedOn, ticketPayload(tc.attempt, tc.meta)))

		slow := eventsOf(hook, eventSlowDelivery)
		if !tc.slow {
			assert.Empty(t, slow, tc.label)
			continue
		}

		require.Len(t, slow, 1, tc.label)
		e := slow[0]
		assert.Equal(t, logrus.ErrorLevel, e.Level, tc.label)
		assert.Equal(t, "sqs ticket delivery exceeded threshold", e.Message, tc.label)
		assert.Equal(t, testTicketID, e.Data[fieldTicketUUID], tc.label)
		assert.Equal(t, testCorrID, e.Data[fieldCorrelationID], tc.label)
		assert.Equal(t, 4, e.Data[fieldOrgID], tc.label)
		assert.Equal(t, tc.attempt+1, e.Data[fieldAttempt], tc.label)
		assert.Equal(t, "2024-10-30T12:00:00.000Z", e.Data[fieldCreatedOn], tc.label)
		assert.Equal(t, "2024-10-30T12:00:02.000Z", e.Data[fieldQueuedOn], tc.label)
		assert.Equal(t, "2024-10-30T12:05:01.000Z", e.Data[fieldSentAt], tc.label)
		assert.Equal(t, int64(2000), e.Data[fieldCreatedToQueuedMs], tc.label)
		assert.Equal(t, int64(299000), e.Data[fieldQueuedToSentMs], tc.label)
		assert.Equal(t, int64(301000), e.Data[fieldCreatedToSentMs], tc.label)
		assert.Equal(t, int64(300000), e.Data[fieldSlowThresholdMs], tc.label)

		if tc.meta != nil {
			assert.Equal(t, "flow", e.Data[fieldOrigin], tc.label)
			assert.Equal(t, "2024-10-30T12:00:10.000Z", e.Data[fieldFirstAttemptAt], tc.label)
			assert.Equal(t, int64(8000), e.Data[fieldQueuedToFirstAttemptMs], tc.label)
			assert.Equal(t, int64(291000), e.Data[fieldFirstAttemptToSentMs], tc.label)
		} else {
			assert.NotContains(t, e.Data, fieldFirstAttemptAt, tc.label)
			assert.NotContains(t, e.Data, fieldQueuedToFirstAttemptMs, tc.label)
			assert.NotContains(t, e.Data, fieldFirstAttemptToSentMs, tc.label)
		}

		// the report goes right after the success it refers to, and carries no personal data
		sent := eventsOf(hook, eventSent)
		require.Len(t, sent, 1, tc.label)
		assertNoPersonalData(t, hook)
	}
}

// the first attempt time is written once, when the first attempt starts, and survives retries untouched
func TestHandleSqsPublish_FirstAttemptIsRecordedOnce(t *testing.T) {
	failing := &sqsFailing{}
	failing.set(true)
	rt, _ := newTestRuntime(t, failing.handler)

	first := time.Date(2024, 10, 30, 12, 0, 7, 0, time.UTC)
	nowFunc = func() time.Time { return first }

	rc := rt.RP.Get()
	defer rc.Close()

	handle(t, rt, leaseTask(t, rt, 4, first.Add(-time.Second), ticketPayload(0, nil)))
	_, err := promoteDue(rc, 10)
	require.NoError(t, err)

	nowFunc = func() time.Time { return first.Add(time.Minute) }
	retry, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, retry)

	p := &PublishTask{}
	require.NoError(t, json.Unmarshal(retry.Task, p))
	require.NotNil(t, p.Meta)
	require.NotNil(t, p.Meta.FirstAttemptAt)
	assert.True(t, first.Equal(*p.Meta.FirstAttemptAt))

	// a second failure keeps carrying the original value
	handle(t, rt, retry)
	_, err = promoteDue(rc, 10)
	require.NoError(t, err)
	again, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, again)

	p = &PublishTask{}
	require.NoError(t, json.Unmarshal(again.Task, p))
	assert.Equal(t, 2, p.Attempt)
	assert.True(t, first.Equal(*p.Meta.FirstAttemptAt))
}

// Q-06: no log of any path (success, retry, giving up, recovery, enqueue failure) carries the URN, the message group
// id or a raw body
func TestSqsPublishLogsNeverCarryPersonalData(t *testing.T) {
	failing := &sqsFailing{}
	rt, _ := newTestRuntime(t, failing.handler)
	rt.Config.SqsPublishMaxAttempts = 1
	hook := captureLogs(t)

	rc := rt.RP.Get()
	defer rc.Close()

	// success
	handle(t, rt, leaseTask(t, rt, 1, time.Now(), ticketPayload(0, nil)))

	// failure with retry, then giving up
	failing.set(true)
	handle(t, rt, leaseTask(t, rt, 2, time.Now(), ticketPayload(0, nil)))
	_, err := promoteDue(rc, 10)
	require.NoError(t, err)
	retry, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	handle(t, rt, retry)

	// a slow delivery, with the failure still on
	failing.set(false)
	nowFunc = func() time.Time { return time.Now().Add(time.Hour * 24 * 365 * 5) }
	handle(t, rt, leaseTask(t, rt, 3, time.Now(), ticketPayload(0, nil)))

	// recovery of an expired lease and giving up on another
	for _, attempt := range []int{0, 1} {
		data, err := json.Marshal(ptr(ticketPayload(attempt, nil)))
		require.NoError(t, err)
		require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 4, Task: data, QueuedOn: time.Now()}))
		_, err = pop(rc, "dead-"+string(rune('a'+attempt)), 20*time.Millisecond)
		require.NoError(t, err)
	}
	time.Sleep(60 * time.Millisecond)
	maintenancePass(rt)

	// an unreadable task
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 5, Task: json.RawMessage(`"junk"`), QueuedOn: time.Now()}))
	var junk *queue.Task
	for junk == nil {
		task, err := taskSource{}.Pop(rt, rc)
		require.NoError(t, err)
		require.NotNil(t, task)
		if task.OrgID == 5 {
			junk = task
		} else {
			_, err = ack(rc, task.OrgID, task.LeaseToken) // the message recovered above, not under test here
			require.NoError(t, err)
		}
	}
	assert.Error(t, handleSqsPublish(context.Background(), rt, junk))

	// enqueue failure
	broken := *rt
	broken.RP = &redis.Pool{Dial: func() (redis.Conn, error) { return nil, errors.New("redis down") }}
	err = EnqueuePublishWithAttributes(&broken, 6, testQueueURL, staticMessage{body: []byte(testTicketMsg), typeStr: "application/json"},
		map[string]string{"MessageGroupId": "p:c:" + testURN, "CorrelationID": testCorrID}, &PublishMeta{TicketUUID: testTicketID, CorrelationID: testCorrID, Origin: "http", CreatedOn: time.Now()})
	assert.Error(t, err)

	// every path was exercised
	for _, event := range []string{eventEnqueued, eventAttempt, eventSent, eventSlowDelivery, eventRetryScheduled, eventDeadLettered, eventRecovered} {
		assert.NotEmpty(t, eventsOf(hook, event), event)
	}
	assert.NotEmpty(t, hook.AllEntries())
	assertNoPersonalData(t, hook)
}
