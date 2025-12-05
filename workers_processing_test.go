package mailroom

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/stretchr/testify/assert"
)

// Test that only send_history tasks are recorded in processing by the worker
func TestWorkerProcessingOnlyForSendHistory(t *testing.T) {
	// Redis pool for tests
	pool := &redis.Pool{
		Wait:      true,
		MaxActive: 4,
		MaxIdle:   2,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", "localhost:6379")
		},
		IdleTimeout: 30 * time.Second,
	}
	rc := pool.Get()
	defer rc.Close()

	q := "unitq"
	org := 2001
	// cleanup keys for our test queue
	rc.Do("del", q+":active", q+":"+itoa(org), q+":"+itoa(org)+":processing")
	keys, _ := redis.Strings(rc.Do("keys", q+":payload:*"))
	for _, k := range keys {
		rc.Do("del", k)
	}

	// runtime with our pool
	rt := &runtime.Runtime{RP: pool}

	// register a blocking test task for a non send_history type
	const nonType = "unit_test_task"
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	prevNon, hasPrevNon := taskFunctions[nonType]
	AddTaskFunction(nonType, func(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
		started <- struct{}{}
		<-release
		return nil
	})
	defer func() {
		if hasPrevNon {
			AddTaskFunction(nonType, prevNon)
		}
	}()

	// create foreman for our test queue
	wg := &sync.WaitGroup{}
	foreman := NewForeman(rt, wg, q, 1)
	foreman.Start()
	defer foreman.Stop()

	// enqueue non send_history task
	err := queue.AddTask(rc, q, nonType, org, map[string]string{"k": "v"}, queue.DefaultPriority)
	assert.NoError(t, err)

	// wait until the task handler starts
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("non send_history task did not start in time")
	}

	// check processing zset is empty for non send_history
	count, err := redis.Int(rc.Do("zcard", q+":"+itoa(org)+":processing"))
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "non send_history should not be tracked in processing")

	// allow the handler to finish
	close(release)
	time.Sleep(200 * time.Millisecond)

	// Now test send_history gets tracked
	started2 := make(chan struct{}, 1)
	release2 := make(chan struct{})
	prevSH, hasPrevSH := taskFunctions[queue.SendHistory]
	AddTaskFunction(queue.SendHistory, func(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
		started2 <- struct{}{}
		<-release2
		return nil
	})
	defer func() {
		if hasPrevSH {
			AddTaskFunction(queue.SendHistory, prevSH)
		}
	}()

	// enqueue send_history task
	err = queue.AddTask(rc, q, queue.SendHistory, org, map[string]any{"ticket_uuid": "u", "contact_id": 1}, queue.DefaultPriority)
	assert.NoError(t, err)

	select {
	case <-started2:
	case <-time.After(3 * time.Second):
		t.Fatal("send_history task did not start in time")
	}

	// should be tracked in processing
	count, err = redis.Int(rc.Do("zcard", q+":"+itoa(org)+":processing"))
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "send_history should be tracked in processing")

	// payload key created
	payloadKeys, err := redis.Strings(rc.Do("keys", q+":payload:*"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(payloadKeys))

	// release and ensure cleanup
	close(release2)
	time.Sleep(300 * time.Millisecond)

	count, err = redis.Int(rc.Do("zcard", q+":"+itoa(org)+":processing"))
	assert.NoError(t, err)
	assert.Equal(t, 0, count, "processing should be cleaned after completion")

	payloadKeys, err = redis.Strings(rc.Do("keys", q+":payload:*"))
	assert.NoError(t, err)
	assert.Equal(t, 0, len(payloadKeys))
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
