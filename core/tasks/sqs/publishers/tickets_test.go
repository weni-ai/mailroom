package publishers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/sqs"
	"github.com/nyaruka/mailroom/runtime"
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

	require.NoError(t, PublishTicketCreated(rt, orgID, msg))

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
}

func TestPublishTicketCreatedWithNoRuntimePool(t *testing.T) {
	rt := &runtime.Runtime{Config: runtime.NewDefaultConfig(), RP: nil}
	// Should not error even if RP is nil; nothing to enqueue
	err := PublishTicketCreated(rt, models.OrgID(1), TicketSQSMessage{})
	assert.NoError(t, err)
}
