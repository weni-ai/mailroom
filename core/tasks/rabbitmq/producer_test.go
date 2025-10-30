package rabbitmq

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime/rmq"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// message that returns a fixed body and type
type staticMessage struct {
	body    []byte
	typeStr string
}

func (m staticMessage) Marshal() ([]byte, error) { return m.body, nil }
func (m staticMessage) ContentType() string      { return m.typeStr }

// message that fails to marshal
type badMessage struct{}

func (m badMessage) Marshal() ([]byte, error) { return nil, errors.New("boom") }
func (m badMessage) ContentType() string      { return "text/plain" }

func TestEnqueuePublish_NilGuards(t *testing.T) {
	// nil runtime
	assert.NoError(t, EnqueuePublish(nil, 1, "ex", "rk", rmq.RawMessage{Body: []byte("{}"), Type: "application/json"}))

	// runtime without RP
	_, rt, _, _ := testsuite.Get()
	rt.RP = nil
	assert.NoError(t, EnqueuePublish(rt, 1, "ex", "rk", rmq.RawMessage{Body: []byte("{}"), Type: "application/json"}))

	// nil message
	_, rt2, _, _ := testsuite.Get()
	assert.NoError(t, EnqueuePublish(rt2, 1, "ex", "rk", nil))
}

func TestEnqueuePublish_EnqueuesTask(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, rp := testsuite.Get()
	_ = ctx

	body := []byte(`{"hello":"world"}`)
	msg := staticMessage{body: body, typeStr: "application/json"}

	require.NoError(t, EnqueuePublish(rt, 7, "tickets.topic", "create", msg))

	rc := rp.Get()
	defer rc.Close()

	task, err := queue.PopNextTask(rc, queue.RabbitmqPublish)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, 7, task.OrgID)
	assert.Equal(t, queue.RabbitmqPublish, task.Type)

	var payload PublishTask
	require.NoError(t, json.Unmarshal(task.Task, &payload))
	assert.Equal(t, "tickets.topic", payload.Exchange)
	assert.Equal(t, "create", payload.RoutingKey)
	assert.Equal(t, "application/json", payload.ContentType)
	assert.Equal(t, json.RawMessage(body), payload.Body)
	assert.Equal(t, 0, payload.Attempt)
}

func TestEnqueuePublish_MarshalError(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	_, rt, _, rp := testsuite.Get()

	// returns error, nothing enqueued
	err := EnqueuePublish(rt, 1, "ex", "rk", badMessage{})
	assert.EqualError(t, err, "boom")

	rc := rp.Get()
	defer rc.Close()

	// ensure queue is empty
	// small sleep to be safe against clock granularity in queue scores
	time.Sleep(5 * time.Millisecond)
	task, err := queue.PopNextTask(rc, queue.RabbitmqPublish)
	require.NoError(t, err)
	assert.Nil(t, task)
}
