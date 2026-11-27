package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testURN       = "whatsapp:5511999999999"
	testTicketID  = "d2f852ec-7b4e-457f-ae7f-f8b243c49ff5"
	testCorrID    = "9688d21d-95aa-4bed-afc7-f31b35731a3d"
	testTicketMsg = `{"ticket_uuid":"d2f852ec-7b4e-457f-ae7f-f8b243c49ff5","contact_urn":"whatsapp:5511999999999","project_uuid":"e7187099-7d38-4f60-955c-325957214c42","channel_uuid":"59d74b86-3e2f-4a93-aece-b05d2fdcde0c","created_on":"2024-10-30T12:00:00Z"}`
)

// sqsFailing is a toggle for fake SQS servers: while true every request is answered with an error
type sqsFailing struct{ v atomic.Bool }

func (f *sqsFailing) set(failing bool) { f.v.Store(failing) }

func (f *sqsFailing) handler(n int, req testsuite.SQSRequest) testsuite.SQSResponse {
	if f.v.Load() {
		return testsuite.SQSResponse{Status: 400}
	}
	return testsuite.SQSResponse{}
}

// newTestRuntime returns a runtime with a clean Redis and an SQS client pointing to a fake endpoint
func newTestRuntime(t *testing.T, handler func(n int, req testsuite.SQSRequest) testsuite.SQSResponse) (*runtime.Runtime, *testsuite.SQSServer) {
	t.Helper()

	testsuite.Reset(testsuite.ResetRedis)
	_, rt, _, _ := testsuite.Get()

	server := testsuite.NewSQSServer(handler)
	t.Cleanup(server.Close)

	client, err := sqsclient.New(sqsclient.ClientConfig{Region: "us-east-1", AccessKeyID: "test", SecretAccessKey: "test", Endpoint: server.URL})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	rt.SQS = client
	rt.Config.SqsPublishMaxAttempts = 3
	rt.Config.SqsPublishDelayIntervalMs = 0
	rt.Config.ProcessingTTL = 120

	// time is injected so slow delivery can be tested without waiting
	prev := nowFunc
	t.Cleanup(func() { nowFunc = prev })

	return rt, server
}

// ticketPayload returns a publish task for a ticket created message, as the publisher builds it
func ticketPayload(attempt int, meta *PublishMeta) PublishTask {
	return PublishTask{
		QueueURL:    testQueueURL,
		ContentType: "application/json",
		Body:        json.RawMessage(testTicketMsg),
		Attributes:  map[string]string{"CorrelationID": testCorrID, "MessageGroupId": "project:channel:" + testURN},
		Attempt:     attempt,
		Meta:        meta,
	}
}

// leaseTask queues the payload for the org and pops it through the sqs source, returning the leased task as a worker gets it
func leaseTask(t *testing.T, rt *runtime.Runtime, orgID int, queuedOn time.Time, p PublishTask) *queue.Task {
	t.Helper()

	data, err := json.Marshal(&p)
	require.NoError(t, err)

	rc := rt.RP.Get()
	defer rc.Close()

	require.NoError(t, enqueue(rc, &queue.Task{Type: queue.SqsPublish, OrgID: orgID, Task: data, QueuedOn: queuedOn}))

	task, err := taskSource{}.Pop(rt, rc)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.NotEmpty(t, task.LeaseToken)
	return task
}

// handle runs the task function like the foreman does
func handle(t *testing.T, rt *runtime.Runtime, task *queue.Task) {
	t.Helper()
	require.NoError(t, handleSqsPublish(context.Background(), rt, task))
}

// captureLogs records everything logged through the standard logger until the test ends
func captureLogs(t *testing.T) *logtest.Hook {
	t.Helper()

	hook := logtest.NewGlobal()
	t.Cleanup(func() { logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks)) })
	return hook
}

// eventsOf returns the captured entries of one event, in order
func eventsOf(hook *logtest.Hook, event string) []*logrus.Entry {
	var found []*logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Data[fieldEvent] == event {
			found = append(found, e)
		}
	}
	return found
}

// assertNoPersonalData fails if any captured entry carries the contact URN, a message group id or a raw task body
func assertNoPersonalData(t *testing.T, hook *logtest.Hook) {
	t.Helper()

	for _, e := range hook.AllEntries() {
		dump := e.Message
		for k, v := range e.Data {
			assert.NotContains(t, []string{"message_group_id", "task", "body", "contact_urn", "attributes"}, k, "forbidden field in: %s", e.Message)
			dump += fmt.Sprintf(" %s=%v", k, v)
		}
		assert.False(t, strings.Contains(dump, "5511999999999"), "contact URN leaked in log: %s", e.Message)
		assert.False(t, strings.Contains(strings.ToLower(dump), "messagegroupid"), "message group id leaked in log: %s", e.Message)
	}
}

// redisKeys returns every key in the test Redis
func redisKeys(t *testing.T, rt *runtime.Runtime) []string {
	t.Helper()

	rc := rt.RP.Get()
	defer rc.Close()
	keys, err := redis.Strings(rc.Do("keys", "*"))
	require.NoError(t, err)
	return keys
}
