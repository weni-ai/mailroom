package publishers

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/sqs"
	"github.com/nyaruka/mailroom/runtime"
	sqsclient "github.com/nyaruka/mailroom/runtime/sqs"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketSQSMessageMarshalAndContentType(t *testing.T) {
	// Use fixed UUIDs for deterministic testing (no testsuite needed for pure serialization test)
	ticketUUID := uuids.UUID("d2f852ec-7b4e-457f-ae7f-f8b243c49ff5")
	projectUUID := uuids.UUID("a3f852ec-7b4e-457f-ae7f-f8b243c49ff6")
	channelUUID := uuids.UUID("b4f852ec-7b4e-457f-ae7f-f8b243c49ff7")
	contactURN := urns.URN("whatsapp:5511999999999")
	createdOn := time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)

	msg := TicketSQSMessage{
		TicketUUID:  ticketUUID,
		ContactURN:  contactURN,
		ProjectUUID: projectUUID,
		ChannelUUID: channelUUID,
		CreatedOn:   createdOn,
	}

	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, "application/json", msg.ContentType())

	// Ensure JSON has expected fields/values
	expected := `{"ticket_uuid":"` + string(ticketUUID) + `","contact_urn":"` + string(contactURN) + `","project_uuid":"` + string(projectUUID) + `","channel_uuid":"` + string(channelUUID) + `","created_on":"` + createdOn.Format(time.RFC3339) + `"}`
	assert.JSONEq(t, expected, string(b))

	// Round-trip to verify stable encoding
	var decoded TicketSQSMessage
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, msg, decoded)
}

func TestPublishTicketCreatedEnqueuesTask(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	_, rt, _, rp := testsuite.Get()

	orgID := models.OrgID(1)

	ticketUUID := uuids.New()
	projectUUID := uuids.New()
	channelUUID := uuids.New()
	contactURN := urns.URN("whatsapp:5511999999999")
	createdOn := time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC)

	msg := TicketSQSMessage{
		TicketUUID:  ticketUUID,
		ContactURN:  contactURN,
		ProjectUUID: projectUUID,
		ChannelUUID: channelUUID,
		CreatedOn:   createdOn,
	}

	require.NoError(t, PublishTicketCreated(rt, orgID, msg, OriginFlow))

	rc := rp.Get()
	defer rc.Close()

	task, err := queue.PopNextTask(rc, queue.SqsPublish)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, int(orgID), task.OrgID)
	assert.Equal(t, queue.SqsPublish, task.Type)

	var payload sqs.PublishTask
	require.NoError(t, json.Unmarshal(task.Task, &payload))
	assert.Equal(t, rt.Config.SqsTicketsQueueURL, payload.QueueURL)
	assert.Equal(t, "application/json", payload.ContentType)

	var body TicketSQSMessage
	require.NoError(t, json.Unmarshal(payload.Body, &body))
	assert.Equal(t, msg, body)

	// the envelope identifies the ticket for logs, without touching the SQS body
	require.NotNil(t, payload.Meta)
	assert.Equal(t, string(ticketUUID), payload.Meta.TicketUUID)
	assert.Equal(t, createdOn, payload.Meta.CreatedOn)
	assert.Equal(t, OriginFlow, payload.Meta.Origin)
	assert.NotEmpty(t, payload.Meta.CorrelationID)
	assert.Equal(t, payload.Meta.CorrelationID, payload.Attributes["CorrelationID"])
	assert.Nil(t, payload.Meta.FirstAttemptAt)
}

func TestPublishTicketCreatedDisabled(t *testing.T) {
	testsuite.Reset(testsuite.ResetAll)
	_, rt, _, rp := testsuite.Get()
	rt.Config.SqsPublishEnabled = false

	require.NoError(t, PublishTicketCreated(rt, models.OrgID(1), TicketSQSMessage{}, OriginHTTP))

	rc := rp.Get()
	defer rc.Close()
	task, err := queue.PopNextTask(rc, queue.SqsPublish)
	require.NoError(t, err)
	assert.Nil(t, task)
}

// sqsContract is what the SQS endpoint receives for one ticket created message
type sqsContract struct {
	Body                   json.RawMessage   `json:"body"`
	MessageAttributes      map[string]string `json:"message_attributes"`
	MessageGroupID         string            `json:"message_group_id,omitempty"`
	MessageDeduplicationID string            `json:"message_deduplication_id,omitempty"`
}

type sqsContractFixture struct {
	Standard sqsContract `json:"standard"`
	FIFO     sqsContract `json:"fifo"`
}

const contractFixturePath = "testdata/ticket_sqs_contract.json"

// publishAndSend publishes a ticket created message to the given queue URL, takes it off the Redis queue and sends it
// to a fake SQS endpoint exactly as the worker does, returning what the endpoint received.
func publishAndSend(t *testing.T, queueURL string) sqsContract {
	testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	server := testsuite.NewSQSServer(nil)
	defer server.Close()

	_, rt, _, rp := testsuite.Get()
	rt.Config.SqsTicketsQueueURL = queueURL

	client, err := sqsclient.New(sqsclient.ClientConfig{Region: "us-east-1", AccessKeyID: "test", SecretAccessKey: "test", Endpoint: server.URL})
	require.NoError(t, err)
	defer client.Close()

	msg := TicketSQSMessage{
		TicketUUID:  uuids.New(),
		ContactURN:  urns.URN("whatsapp:5511999999999"),
		ProjectUUID: uuids.New(),
		ChannelUUID: uuids.New(),
		CreatedOn:   time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, publishTicketCreated(rt, models.OrgID(1), msg))

	rc := rp.Get()
	defer rc.Close()

	task, err := queue.PopNextTask(rc, queue.SqsPublish)
	require.NoError(t, err)
	require.NotNil(t, task)

	var payload sqs.PublishTask
	require.NoError(t, json.Unmarshal(task.Task, &payload))
	require.Equal(t, queueURL, payload.QueueURL)

	require.NoError(t, client.SendToWithAttributes(payload.QueueURL, sqsclient.RawMessage{Body: payload.Body, Type: payload.ContentType}, payload.Attributes))

	reqs := server.Requests()
	require.Len(t, reqs, 1)

	return sqsContract{
		Body:                   json.RawMessage(reqs[0].MessageBody),
		MessageAttributes:      reqs[0].MessageAttributes,
		MessageGroupID:         reqs[0].MessageGroupID,
		MessageDeduplicationID: reqs[0].MessageDeduplicationID,
	}
}

// TestTicketSQSContract guards the external contract (body and attributes) against the fixture captured before the
// reliable publish change (spec FR-008). Run with UPDATE_SQS_CONTRACT=1 only to regenerate it deliberately.
func TestTicketSQSContract(t *testing.T) {
	got := sqsContractFixture{
		Standard: publishAndSend(t, "http://sqs.us-east-1.localhost.localstack.cloud:4566/000000000000/flows-tickets"),
		FIFO:     publishAndSend(t, "http://sqs.us-east-1.localhost.localstack.cloud:4566/000000000000/flows-tickets.fifo"),
	}

	if os.Getenv("UPDATE_SQS_CONTRACT") != "" {
		data, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll("testdata", 0755))
		require.NoError(t, os.WriteFile(contractFixturePath, append(data, '\n'), 0644))
		return
	}

	raw, err := os.ReadFile(contractFixturePath)
	require.NoError(t, err)

	var want sqsContractFixture
	require.NoError(t, json.Unmarshal(raw, &want))

	for name, pair := range map[string][2]sqsContract{"standard": {want.Standard, got.Standard}, "fifo": {want.FIFO, got.FIFO}} {
		assert.JSONEq(t, string(pair[0].Body), string(pair[1].Body), name)
		assert.Equal(t, pair[0].MessageAttributes, pair[1].MessageAttributes, name)
		assert.Equal(t, pair[0].MessageGroupID, pair[1].MessageGroupID, name)
		assert.Equal(t, pair[0].MessageDeduplicationID, pair[1].MessageDeduplicationID, name)
	}
}

// publishTicketCreated wraps the publisher so tests keep a single call site
func publishTicketCreated(rt *runtime.Runtime, orgID models.OrgID, msg TicketSQSMessage) error {
	return PublishTicketCreated(rt, orgID, msg, OriginHTTP)
}

func TestPublishTicketCreatedWithNoRuntimePool(t *testing.T) {
	rt := &runtime.Runtime{Config: runtime.NewDefaultConfig(), RP: nil}
	// Should not error even if RP is nil; nothing to enqueue
	err := PublishTicketCreated(rt, models.OrgID(1), TicketSQSMessage{}, OriginHTTP)
	assert.NoError(t, err)
}
