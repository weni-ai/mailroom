package mailroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

// Test that only send_history tasks are recorded in processing by the worker
func TestWorkerProcessingOnlyForSendHistory(t *testing.T) {
	_, rt, _, _ := testsuite.Get()
	rc := rt.RP.Get()
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

// sqsTaskBody is a sqs_publish task body as it travels in the queue: it carries the contact URN
const sqsTaskBody = `{"queue_url":"http://sqs/q","body":{"contact_urn":"whatsapp:5511999999999"},"attributes":{"MessageGroupId":"p:c:whatsapp:5511999999999"}}`

// Q-06: whatever happens to a sqs_publish task in the foreman (panic, error, long run), its body, with the URN in it,
// never reaches the logs, while other task types keep logging theirs
func TestWorkerNeverLogsSqsPublishBody(t *testing.T) {
	_, rt, _, _ := testsuite.Get()
	rc := rt.RP.Get()
	defer rc.Close()

	q := "unitq_sqs_redaction"
	org := 2002
	rc.Do("del", q+":active", q+":"+itoa(org))

	hook := logtest.NewGlobal()
	defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))

	// handler of the sqs type: panics on the first task, fails on the second
	calls := 0
	done := make(chan struct{}, 4)
	prev, hadPrev := taskFunctions[queue.SqsPublish]
	AddTaskFunction(queue.SqsPublish, func(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
		defer func() { done <- struct{}{} }()
		calls++
		if calls == 1 {
			panic("handler exploded")
		}
		return errors.New("send failed")
	})
	defer func() {
		if hadPrev {
			AddTaskFunction(queue.SqsPublish, prev)
		} else {
			delete(taskFunctions, queue.SqsPublish)
		}
	}()

	wg := &sync.WaitGroup{}
	foreman := NewForeman(rt, wg, q, 1)
	foreman.Start()
	defer foreman.Stop()

	for i := 0; i < 2; i++ {
		assert.NoError(t, queue.AddTask(rc, q, queue.SqsPublish, org, json.RawMessage(sqsTaskBody), queue.DefaultPriority))
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("sqs task was not handled in time")
		}
		time.Sleep(100 * time.Millisecond) // let the worker log and release
	}

	// the long running path logs through the same helper
	entry := taskLog(logrus.WithField("queue", q), &queue.Task{Type: queue.SqsPublish, OrgID: org, Task: json.RawMessage(sqsTaskBody)})
	entry.Warn("long running task")

	var sawPanic, sawError bool
	for _, e := range hook.AllEntries() {
		dump := e.Message
		for k, v := range e.Data {
			dump += fmt.Sprintf(" %s=%v", k, v)
		}
		assert.NotContains(t, e.Data, "task", e.Message)
		assert.False(t, strings.Contains(dump, "5511999999999"), "URN leaked: %s", e.Message)
		assert.False(t, strings.Contains(strings.ToLower(dump), "messagegroupid"), "message group id leaked: %s", e.Message)
		assert.False(t, strings.Contains(dump, "message_group_id"), "message group id leaked: %s", e.Message)

		sawPanic = sawPanic || strings.Contains(e.Message, "panic handling task")
		sawError = sawError || e.Message == "error running task"
	}
	assert.True(t, sawPanic, "the panic must still be logged")
	assert.True(t, sawError, "the error must still be logged")

	// tasks of other types keep their body in the logs
	other := taskLog(logrus.WithField("queue", q), &queue.Task{Type: "some_other_task", Task: json.RawMessage(`{"k":"v"}`)})
	assert.Equal(t, `{"k":"v"}`, other.Data["task"])
}
