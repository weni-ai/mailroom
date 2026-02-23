package sqs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/queue"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"
	"github.com/nyaruka/mailroom/testsuite"

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
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()

	task := &queue.Task{Type: queue.SqsPublish, OrgID: 1, Task: json.RawMessage(`not-json`)}
	err := handleSqsPublish(ctx, rt, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshalling sqs publish task")
}

func TestHandleSqsPublish_NoClient(t *testing.T) {
	ctx := context.Background()
	_, rt, _, _ := testsuite.Get()
	rt.SQS = nil

	p := PublishTask{QueueURL: "http://localhost:4566/000000000000/test-queue", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 0}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.SqsPublish, OrgID: 1, Task: body}

	err := handleSqsPublish(ctx, rt, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sqs client not initialized")
}

func TestHandleSqsPublish_RequeueOnFailure(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, rp := testsuite.Get()

	// make sqs client present but non-functional to force failure
	rt.SQS = &sqsclient.Client{}
	rt.Config.SqsPublishMaxAttempts = 2
	rt.Config.SqsPublishDelayIntervalMs = 0

	p := PublishTask{QueueURL: "http://localhost:4566/000000000000/test-queue", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 0}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.SqsPublish, OrgID: 9, Task: body}

	// should schedule a retry and return nil
	err := handleSqsPublish(ctx, rt, task)
	assert.NoError(t, err)

	// wait briefly for async requeue
	rc := rp.Get()
	defer rc.Close()

	var next *queue.Task
	for i := 0; i < 20; i++ {
		time.Sleep(5 * time.Millisecond)
		next, _ = queue.PopNextTask(rc, queue.SqsPublish)
		if next != nil {
			break
		}
	}
	require.NotNil(t, next)
	assert.Equal(t, 9, next.OrgID)

	var np PublishTask
	require.NoError(t, json.Unmarshal(next.Task, &np))
	assert.Equal(t, 1, np.Attempt)
	assert.Equal(t, "http://localhost:4566/000000000000/test-queue", np.QueueURL)
}

func TestHandleSqsPublish_RequeueOnFailureWithAttributes(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, rp := testsuite.Get()

	// make sqs client present but non-functional to force failure
	rt.SQS = &sqsclient.Client{}
	rt.Config.SqsPublishMaxAttempts = 2
	rt.Config.SqsPublishDelayIntervalMs = 0

	attrs := map[string]string{"EventType": "test"}
	p := PublishTask{
		QueueURL:    "http://localhost:4566/000000000000/test-queue",
		ContentType: "application/json",
		Body:        json.RawMessage(`{"a":1}`),
		Attributes:  attrs,
		Attempt:     0,
	}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.SqsPublish, OrgID: 10, Task: body}

	// should schedule a retry and return nil
	err := handleSqsPublish(ctx, rt, task)
	assert.NoError(t, err)

	// wait briefly for async requeue
	rc := rp.Get()
	defer rc.Close()

	var next *queue.Task
	for i := 0; i < 20; i++ {
		time.Sleep(5 * time.Millisecond)
		next, _ = queue.PopNextTask(rc, queue.SqsPublish)
		if next != nil {
			break
		}
	}
	require.NotNil(t, next)
	assert.Equal(t, 10, next.OrgID)

	var np PublishTask
	require.NoError(t, json.Unmarshal(next.Task, &np))
	assert.Equal(t, 1, np.Attempt)
	assert.Equal(t, attrs, np.Attributes)
}

func TestHandleSqsPublish_MaxRetriesReached(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	ctx, rt, _, _ := testsuite.Get()

	// non-functional client to force failure
	rt.SQS = &sqsclient.Client{}
	rt.Config.SqsPublishMaxAttempts = 1
	rt.Config.SqsPublishDelayIntervalMs = 0

	p := PublishTask{QueueURL: "http://localhost:4566/000000000000/test-queue", ContentType: "application/json", Body: json.RawMessage(`{"a":1}`), Attempt: 1}
	body, _ := json.Marshal(&p)
	task := &queue.Task{Type: queue.SqsPublish, OrgID: 3, Task: body}

	err := handleSqsPublish(ctx, rt, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max retries reached for sqs publish")
}
