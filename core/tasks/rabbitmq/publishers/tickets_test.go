package publishers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/rabbitmq"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/testsuite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicketRMQMessageMarshalAndContentType(t *testing.T) {

	testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	ticketUUID := flows.TicketUUID(uuids.New())
	contactUUID := flows.ContactUUID(uuids.New())
	projectUUID := uuids.New()
	createdOn := time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)

	msg := TicketRMQMessage{
		UUID:         ticketUUID,
		ContactUUID:  contactUUID,
		ProjectUUID:  projectUUID,
		TicketerType: "zendesk",
		CreatedOn:    createdOn,
	}

	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, "application/json", msg.ContentType())

	// Ensure JSON has expected fields/values
	expected := `{"uuid":"` + string(ticketUUID) + `","contact_uuid":"` + string(contactUUID) + `","project_uuid":"` + string(projectUUID) + `","type":"zendesk","created_on":"` + createdOn.Format(time.RFC3339) + `"}`
	assert.JSONEq(t, expected, string(b))

	// Round-trip to verify stable encoding
	var decoded TicketRMQMessage
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, msg, decoded)
}

func TestPublishTicketCreatedEnqueuesTask(t *testing.T) {

	testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	_, rt, _, rp := testsuite.Get()

	orgID := models.OrgID(1)

	ticketUUID := flows.TicketUUID(uuids.New())
	contactUUID := flows.ContactUUID(uuids.New())
	projectUUID := uuids.New()
	createdOn := time.Date(2024, 10, 30, 12, 0, 0, 0, time.UTC)

	msg := TicketRMQMessage{
		UUID:         ticketUUID,
		ContactUUID:  contactUUID,
		ProjectUUID:  projectUUID,
		TicketerType: "zendesk",
		CreatedOn:    createdOn,
	}

	require.NoError(t, PublishTicketCreated(rt, orgID, msg))

	rc := rp.Get()
	defer rc.Close()

	task, err := queue.PopNextTask(rc, queue.RabbitmqPublish)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, int(orgID), task.OrgID)
	assert.Equal(t, queue.RabbitmqPublish, task.Type)

	var payload rabbitmq.PublishTask
	require.NoError(t, json.Unmarshal(task.Task, &payload))
	assert.Equal(t, rt.Config.RabbitmqTicketsExchange, payload.Exchange)
	assert.Equal(t, rt.Config.RabbitmqTicketsRoutingKey, payload.RoutingKey)
	assert.Equal(t, "application/json", payload.ContentType)

	var body TicketRMQMessage
	require.NoError(t, json.Unmarshal(payload.Body, &body))
	assert.Equal(t, msg, body)
}

func TestPublishTicketCreatedWithNoRuntimePool(t *testing.T) {
	rt := &runtime.Runtime{Config: runtime.NewDefaultConfig(), RP: nil}
	// Should not error even if RP is nil; nothing to enqueue
	err := PublishTicketCreated(rt, models.OrgID(1), TicketRMQMessage{})
	assert.NoError(t, err)
}
