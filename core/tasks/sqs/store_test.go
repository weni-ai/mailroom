package sqs

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testQueueURL = "http://localhost:4566/000000000000/test-queue"

// newStoreTask builds a task like the producer does
func newStoreTask(t *testing.T, orgID int, queuedOn time.Time, attempt int) *queue.Task {
	t.Helper()

	payload, err := json.Marshal(&PublishTask{
		QueueURL:    testQueueURL,
		ContentType: "application/json",
		Body:        json.RawMessage(`{"ticket_uuid":"d2f852ec-7b4e-457f-ae7f-f8b243c49ff5","contact_urn":"whatsapp:5511999999999","created_on":"2024-10-30T12:00:00Z"}`),
		Attributes:  map[string]string{"CorrelationID": "corr-1"},
		Attempt:     attempt,
	})
	require.NoError(t, err)

	return &queue.Task{Type: queue.SqsPublish, OrgID: orgID, Task: payload, QueuedOn: queuedOn}
}

// storeRedis returns a clean Redis connection for store tests
func storeRedis(t *testing.T) redis.Conn {
	t.Helper()

	testsuite.Reset(testsuite.ResetRedis)
	_, _, _, rp := testsuite.Get()
	rc := rp.Get()
	t.Cleanup(func() { rc.Close() })
	return rc
}

func zcard(t *testing.T, rc redis.Conn, key string) int {
	t.Helper()
	n, err := redis.Int(rc.Do("zcard", key))
	require.NoError(t, err)
	return n
}

func hlen(t *testing.T, rc redis.Conn, key string) int {
	t.Helper()
	n, err := redis.Int(rc.Do("hlen", key))
	require.NoError(t, err)
	return n
}

// activeScore returns the number of leases of an org, or -1 if the org is not in active
func activeScore(t *testing.T, rc redis.Conn, orgID int) float64 {
	t.Helper()
	score, err := redis.Float64(rc.Do("zscore", keyActive, orgID))
	if err == redis.ErrNil {
		return -1
	}
	require.NoError(t, err)
	return score
}

func TestStoreEnqueueIsAtomic(t *testing.T) {
	rc := storeRedis(t)

	require.NoError(t, enqueue(rc, newStoreTask(t, 7, time.Now(), 0)))

	// the message and its org become visible together
	assert.Equal(t, 1, zcard(t, rc, orgKey(7)))
	assert.Equal(t, float64(0), activeScore(t, rc, 7))

	// enqueuing again does not disturb the lease count of the org
	token := "token-1"
	task, err := pop(rc, token, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, float64(1), activeScore(t, rc, 7))

	require.NoError(t, enqueue(rc, newStoreTask(t, 7, time.Now(), 0)))
	assert.Equal(t, float64(1), activeScore(t, rc, 7))
	assert.Equal(t, 1, zcard(t, rc, orgKey(7)))
}

func TestStorePopLeasesTask(t *testing.T) {
	rc := storeRedis(t)

	// nothing to pop
	task, err := pop(rc, "token-0", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, task)

	queuedOn := time.Now().Add(-time.Minute)
	require.NoError(t, enqueue(rc, newStoreTask(t, 3, queuedOn, 0)))

	task, err = pop(rc, "token-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, "token-1", task.LeaseToken)
	assert.Equal(t, 3, task.OrgID)
	assert.True(t, queuedOn.Equal(task.QueuedOn), "queued_on must survive the round trip")

	// it left the eligible queue and is leased, with its payload kept for recovery
	assert.Equal(t, 0, zcard(t, rc, orgKey(3)))
	assert.Equal(t, 1, zcard(t, rc, keyInflight))
	assert.Equal(t, 1, hlen(t, rc, keyInflightValue))
	assert.Equal(t, float64(1), activeScore(t, rc, 3))

	// the deadline is on the Redis clock, ttl from now
	deadline, err := redis.Int64(rc.Do("zscore", keyInflight, "token-1"))
	require.NoError(t, err)
	now, err := redisNowMs(rc)
	require.NoError(t, err)
	assert.InDelta(t, now+time.Minute.Milliseconds(), deadline, 5000)

	// and there is nothing left to pop
	task, err = pop(rc, "token-2", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, task)
}

func TestStoreAckGuardAndActiveNeverNegative(t *testing.T) {
	rc := storeRedis(t)

	require.NoError(t, enqueue(rc, newStoreTask(t, 5, time.Now(), 0)))
	task, err := pop(rc, "token-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, task)

	ok, err := ack(rc, 5, "token-1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.Equal(t, float64(0), activeScore(t, rc, 5))

	// a second ack of the same lease, or of an unknown one, does nothing and never drives active below zero
	for _, token := range []string{"token-1", "never-leased"} {
		ok, err = ack(rc, 5, token)
		require.NoError(t, err)
		assert.False(t, ok)
	}
	assert.Equal(t, float64(0), activeScore(t, rc, 5))
}

func TestStoreRescheduleThenPromote(t *testing.T) {
	rc := storeRedis(t)

	queuedOn := time.Now().Add(-time.Hour)
	require.NoError(t, enqueue(rc, newStoreTask(t, 4, queuedOn, 0)))
	task, err := pop(rc, "token-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, task)

	next := newStoreTask(t, 4, task.QueuedOn, 1)
	at, ok, err := reschedule(rc, 4, "token-1", next, time.Hour)
	require.NoError(t, err)
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(time.Hour), at, 5*time.Second)

	// it left the lease and waits in delayed; the org slot was released
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.Equal(t, 1, zcard(t, rc, keyDelayed))
	assert.Equal(t, 0, zcard(t, rc, orgKey(4)))
	assert.Equal(t, float64(0), activeScore(t, rc, 4))

	// not due yet: promote does nothing
	n, err := promoteDue(rc, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 1, zcard(t, rc, keyDelayed))

	// a retry without delay is promoted back to its org queue, with its original score
	require.NoError(t, enqueue(rc, newStoreTask(t, 4, time.Now(), 0)))
	_, err = pop(rc, "token-2", time.Minute)
	require.NoError(t, err)
	_, ok, err = reschedule(rc, 4, "token-2", newStoreTask(t, 4, queuedOn.Add(time.Minute), 1), 0)
	require.NoError(t, err)
	require.True(t, ok)

	n, err = promoteDue(rc, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, zcard(t, rc, keyDelayed)) // the hour-long one is still waiting
	assert.Equal(t, 1, zcard(t, rc, orgKey(4)))

	scores, err := redis.Strings(rc.Do("zrange", orgKey(4), 0, -1, "withscores"))
	require.NoError(t, err)
	assert.Equal(t, queueScore(queuedOn.Add(time.Minute)), scores[1])
}

func TestStoreRescheduleWithoutLeaseDoesNothing(t *testing.T) {
	rc := storeRedis(t)

	_, ok, err := reschedule(rc, 4, "never-leased", newStoreTask(t, 4, time.Now(), 1), 0)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, 0, zcard(t, rc, keyDelayed))
	assert.Equal(t, float64(-1), activeScore(t, rc, 4), "no org may be created for a lost lease")
}

func TestStorePopOrdersByQueuedOnWithinOrg(t *testing.T) {
	rc := storeRedis(t)

	base := time.Now().Add(-time.Hour)

	// enqueued out of order, the oldest queued_on must come out first
	for _, offset := range []time.Duration{3 * time.Second, time.Second, 2 * time.Second} {
		require.NoError(t, enqueue(rc, newStoreTask(t, 9, base.Add(offset), 0)))
	}

	var got []time.Time
	for i := 0; i < 3; i++ {
		task, err := pop(rc, "token-"+string(rune('a'+i)), time.Minute)
		require.NoError(t, err)
		require.NotNil(t, task)
		got = append(got, task.QueuedOn)
		_, err = ack(rc, 9, task.LeaseToken)
		require.NoError(t, err)
	}

	assert.True(t, got[0].Equal(base.Add(time.Second)))
	assert.True(t, got[1].Equal(base.Add(2*time.Second)))
	assert.True(t, got[2].Equal(base.Add(3*time.Second)))
}

func TestStorePopKeepsOrgsIndependent(t *testing.T) {
	rc := storeRedis(t)

	base := time.Now().Add(-time.Hour)
	require.NoError(t, enqueue(rc, newStoreTask(t, 1, base, 0)))
	require.NoError(t, enqueue(rc, newStoreTask(t, 1, base.Add(time.Second), 0)))
	require.NoError(t, enqueue(rc, newStoreTask(t, 2, base.Add(2*time.Second), 0)))

	// org 1 has a lease, so org 2 (no leases) is served next even though its message is newer
	first, err := pop(rc, "token-1", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 1, first.OrgID)

	second, err := pop(rc, "token-2", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2, second.OrgID)

	// after org 2 is done, org 1 keeps its own order
	_, err = ack(rc, 2, "token-2")
	require.NoError(t, err)
	third, err := pop(rc, "token-3", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, 1, third.OrgID)
	assert.True(t, third.QueuedOn.Equal(base.Add(time.Second)))
}

func TestStoreOnlyOneActorWinsALease(t *testing.T) {
	rc := storeRedis(t)
	_, _, _, rp := testsuite.Get()

	require.NoError(t, enqueue(rc, newStoreTask(t, 6, time.Now(), 0)))
	task, err := pop(rc, "token-1", 50*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, task)

	// let the lease expire, so the worker (ack and reschedule) and the recovery pass all want the same token
	time.Sleep(120 * time.Millisecond)

	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	run := func(f func(c redis.Conn) bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := rp.Get()
			defer c.Close()
			if f(c) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}

	for i := 0; i < 3; i++ {
		run(func(c redis.Conn) bool {
			ok, err := ack(c, 6, "token-1")
			assert.NoError(t, err)
			return ok
		})
		run(func(c redis.Conn) bool {
			_, ok, err := reschedule(c, 6, "token-1", newStoreTask(t, 6, task.QueuedOn, 1), 0)
			assert.NoError(t, err)
			return ok
		})
		run(func(c redis.Conn) bool {
			recovered, err := reclaimExpired(c, 10, 10)
			assert.NoError(t, err)
			return len(recovered) == 1
		})
	}
	wg.Wait()

	assert.Equal(t, 1, wins, "exactly one actor may take over a lease")
	assert.Equal(t, 0, zcard(t, rc, keyInflight))
	assert.Equal(t, 0, hlen(t, rc, keyInflightValue))
	assert.GreaterOrEqual(t, activeScore(t, rc, 6), float64(0))
}
