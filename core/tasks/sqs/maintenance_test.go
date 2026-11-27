package sqs

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/queue"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Q-08(a): a lease that has not expired belongs to a live worker and must not be touched
func TestMaintenance_ValidLeaseIsNotRecovered(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)
	hook := captureLogs(t)

	task := leaseTask(t, rt, 5, time.Now(), ticketPayload(0, nil))

	rc := rt.RP.Get()
	defer rc.Close()

	maintenancePass(rt)

	assert.Equal(t, 1, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, zcard(t, rc, orgKey(5)))
	assert.Equal(t, float64(1), activeScore(t, rc, 5))
	assert.Empty(t, eventsOf(hook, eventRecovered))

	// even asked directly, the script refuses to take an unexpired lease
	n, err := scriptReclaim.Do(rc, keyInflight, keyInflightValue, keyActive, orgKey(5), task.LeaseToken, 5, "x", "1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// Q-08(b): an expired lease is recovered with one more attempt and the same body and attributes
func TestMaintenance_ExpiredLeaseIsRecovered(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)
	hook := captureLogs(t)

	rc := rt.RP.Get()
	defer rc.Close()

	original := ticketPayload(0, &PublishMeta{TicketUUID: testTicketID, CorrelationID: testCorrID, Origin: "http", CreatedOn: time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC)})
	data, err := json.Marshal(&original)
	require.NoError(t, err)

	queuedOn := time.Now().Add(-time.Minute)
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 5, Task: data, QueuedOn: queuedOn}))

	// the worker takes it and dies: nobody ever acks
	leased, err := pop(rc, "token-dead-worker", 50*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, leased)
	time.Sleep(120 * time.Millisecond)

	maintenancePass(rt)

	// it is back in its org queue, not leased anymore
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.Equal(t, 1, zcard(t, rc, orgKey(5)))

	recovered := eventsOf(hook, eventRecovered)
	require.Len(t, recovered, 1)
	assert.Equal(t, logrus.WarnLevel, recovered[0].Level)
	assert.Equal(t, 1, recovered[0].Data[fieldRecoveredCount])
	items := recovered[0].Data[fieldRecovered].([]map[string]interface{})
	require.Len(t, items, 1)
	assert.Equal(t, testTicketID, items[0][fieldTicketUUID])
	assert.Equal(t, testCorrID, items[0][fieldCorrelationID])
	assert.Equal(t, 5, items[0][fieldOrgID])
	assert.Equal(t, testQueueURL, items[0][fieldQueueURL])

	// and it is sent later with the same body and attributes
	again, err := pop(rc, "token-2", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.True(t, queuedOn.Equal(again.QueuedOn), "queued_on must not change")

	p := &PublishTask{}
	require.NoError(t, json.Unmarshal(again.Task, p))
	assert.Equal(t, 1, p.Attempt, "a recovered lease counts as an attempt")
	assert.JSONEq(t, string(original.Body), string(p.Body))
	assert.Equal(t, original.Attributes, p.Attributes)
	assert.Equal(t, original.ContentType, p.ContentType)
	assert.Equal(t, original.QueueURL, p.QueueURL)
}

// a message whose worker keeps dying is not recovered forever
func TestMaintenance_ExpiredLeaseWithoutAttemptsLeftIsGivenUp(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)
	rt.Config.SqsPublishMaxAttempts = 2
	hook := captureLogs(t)

	rc := rt.RP.Get()
	defer rc.Close()

	data, err := json.Marshal(ptr(ticketPayload(2, nil)))
	require.NoError(t, err)
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 5, Task: data, QueuedOn: time.Now()}))
	_, err = pop(rc, "token-1", 50*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(120 * time.Millisecond)

	maintenancePass(rt)

	assert.Equal(t, 0, zcard(t, rc, orgKey(5)))
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.GreaterOrEqual(t, activeScore(t, rc, 5), float64(0))
	assert.Empty(t, eventsOf(hook, eventRecovered))

	dead := eventsOf(hook, eventDeadLettered)
	require.Len(t, dead, 1)
	assert.Equal(t, logrus.ErrorLevel, dead[0].Level)
	assert.Equal(t, testTicketID, dead[0].Data[fieldTicketUUID])
	assert.Equal(t, 3, dead[0].Data[fieldAttempt])
	assert.NotContains(t, dead[0].Data, fieldAttemptedAt, "the time of the lost attempt is unknown")
}

// Q-04: a due retry returns to the position of its original queued_on, ahead of anything queued after it
func TestMaintenance_PromotedRetryKeepsItsPlace(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)

	rc := rt.RP.Get()
	defer rc.Close()

	first := time.Now().Add(-time.Minute)
	later := first.Add(30 * time.Second)

	task := leaseTask(t, rt, 4, first, ticketPayload(0, nil))

	// the first message fails and waits for its retry...
	next := *task
	next.LeaseToken = ""
	next.Task, _ = json.Marshal(ptr(ticketPayload(1, nil)))
	_, ok, err := reschedule(rc, 4, task.LeaseToken, &next, 0)
	require.NoError(t, err)
	require.True(t, ok)

	// ...while another one is queued behind it, in the same org
	require.NoError(t, enqueue(rc, newStoreTask(t, 4, later, 0)))

	maintenancePass(rt)

	one, err := pop(rc, "token-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, one)
	two, err := pop(rc, "token-2", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, two)

	assert.True(t, first.Equal(one.QueuedOn), "the retry goes first")
	assert.True(t, later.Equal(two.QueuedOn))

	p := &PublishTask{}
	require.NoError(t, json.Unmarshal(one.Task, p))
	assert.Equal(t, 1, p.Attempt)
}

func TestMaintenance_NothingToRecoverLogsNothing(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)
	hook := captureLogs(t)

	maintenancePass(rt)

	assert.Empty(t, eventsOf(hook, eventRecovered))
}

// FR-006: at startup, before the foremen start, expired leases are put back
func TestMaintenance_StartRecoversBeforeReturning(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)

	rc := rt.RP.Get()
	defer rc.Close()

	data, err := json.Marshal(ptr(ticketPayload(0, nil)))
	require.NoError(t, err)
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 5, Task: data, QueuedOn: time.Now()}))
	_, err = pop(rc, "token-dead-process", 50*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(120 * time.Millisecond)

	quit := make(chan bool)
	wg := &sync.WaitGroup{}
	require.NoError(t, startMaintenance(rt, wg, quit))

	// recovered synchronously, no waiting for the first tick
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 1, zcard(t, rc, orgKey(5)))

	// the loop keeps running: a retry that becomes due is promoted by it
	next := newStoreTask(t, 6, time.Now(), 1)
	_, err = pop(rc, "unused", time.Minute) // take the recovered one so org 5 stays out of the way
	require.NoError(t, err)
	require.NoError(t, enqueue(rc, newStoreTask(t, 6, time.Now(), 0)))
	leased, err := pop(rc, "token-6", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, leased)
	_, ok, err := reschedule(rc, 6, "token-6", next, 0)
	require.NoError(t, err)
	require.True(t, ok)

	assert.Eventually(t, func() bool { return zcard(t, rc, orgKey(6)) == 1 }, 5*time.Second, 50*time.Millisecond)

	close(quit)
	wg.Wait()
}

func TestMaintenance_DisabledWithoutSQS(t *testing.T) {
	rt, _ := newTestRuntime(t, nil)

	rc := rt.RP.Get()
	defer rc.Close()

	data, err := json.Marshal(ptr(ticketPayload(0, nil)))
	require.NoError(t, err)
	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: 5, Task: data, QueuedOn: time.Now()}))
	_, err = pop(rc, "token", 50*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(120 * time.Millisecond)

	for _, disable := range []func(){
		func() { rt.Config.SqsPublishEnabled = false },
		func() { rt.Config.SqsPublishEnabled = true; rt.SQS = nil },
	} {
		disable()

		quit := make(chan bool)
		wg := &sync.WaitGroup{}
		require.NoError(t, startMaintenance(rt, wg, quit))
		close(quit)
		wg.Wait()

		assert.Equal(t, 1, zcard(t, rc, keyInflight), "nothing may be recovered when SQS publishing is off")
	}
}

func ptr[T any](v T) *T { return &v }
