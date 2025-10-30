package rabbitmq

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime/rmq"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRabbitmqPublish_WrongType(t *testing.T) {
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()

	task := &queue.Task{Type: "other", OrgID: 1, Task: json.RawMessage(`{}`)}
	err := handleRabbitmqPublish(ctx, rt, task)
	assert.Error(t, err)
}

func TestHandleRabbitmqPublish_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()

	task := &queue.Task{Type: queue.RabbitmqPublish, OrgID: 1, Task: json.RawMessage(`not-json`)}
	err := handleRabbitmqPublish(ctx, rt, task)
	assert.Error(t, err)
}

func TestHandleRabbitmqPublish_NoClient(t *testing.T) {
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()
	rt.Rabbitmq = nil

	p := PublishTask{Exchange: "ex", RoutingKey: "rk", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 0}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.RabbitmqPublish, OrgID: 1, Task: body}

	err := handleRabbitmqPublish(ctx, rt, task)
	assert.Error(t, err)
}

func TestHandleRabbitmqPublish_RequeueOnFailure(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, rp := testsuite.Get()

	// make rmq client present but non-functional to force failure
	rt.Rabbitmq = &rmq.Client{}
	rt.Config.RabbitmqPublishMaxAttempts = 2
	rt.Config.RabbitmqPublishDelayIntervalMs = 0

	p := PublishTask{Exchange: "ex", RoutingKey: "rk", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 0}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.RabbitmqPublish, OrgID: 9, Task: body}

	// should schedule a retry and return nil
	err := handleRabbitmqPublish(ctx, rt, task)
	assert.NoError(t, err)

	// wait briefly for async requeue
	rc := rp.Get()
	defer rc.Close()

	var next *queue.Task
	for i := 0; i < 20; i++ {
		time.Sleep(5 * time.Millisecond)
		next, _ = queue.PopNextTask(rc, queue.RabbitmqPublish)
		if next != nil {
			break
		}
	}
	require.NotNil(t, next)
	assert.Equal(t, 9, next.OrgID)

	var np PublishTask
	require.NoError(t, json.Unmarshal(next.Task, &np))
	assert.Equal(t, 1, np.Attempt)
	assert.Equal(t, "ex", np.Exchange)
	assert.Equal(t, "rk", np.RoutingKey)
}

func TestHandleRabbitmqPublish_MaxRetriesReached(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, _ := testsuite.Get()

	// non-functional client to force failure
	rt.Rabbitmq = &rmq.Client{}
	rt.Config.RabbitmqPublishMaxAttempts = 1
	rt.Config.RabbitmqPublishDelayIntervalMs = 0

	p := PublishTask{Exchange: "ex", RoutingKey: "rk", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 1}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.RabbitmqPublish, OrgID: 3, Task: body}

	err := handleRabbitmqPublish(ctx, rt, task)
	assert.Error(t, err)
}
