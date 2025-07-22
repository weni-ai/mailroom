package tickets_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/tickets"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/null"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendTicketHistoryQueueAndTask(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()
	rc := rp.Get()
	defer rc.Close()

	defer testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	mockTicketer := setupMockTicketer(t, db)
	modelTicket := createTestTicket(t, db, mockTicketer)

	// Test successful send
	sendHistoryCalled = false
	mockError = nil

	err := tickets.QueueSendHistory(rc, modelTicket, testdata.Cathy.ID)
	assert.NoError(t, err)

	task, err := queue.PopNextTask(rc, queue.HandlerQueue)
	require.NoError(t, err)
	require.NotNil(t, task)

	assert.Equal(t, queue.SendHistory, task.Type)
	assert.Equal(t, int(testdata.Org1.ID), task.OrgID)

	var historyTask tickets.SendHistoryTask
	err = json.Unmarshal(task.Task, &historyTask)
	assert.NoError(t, err)
	assert.Equal(t, modelTicket.UUID(), historyTask.TicketUUID)
	assert.Equal(t, testdata.Cathy.ID, historyTask.ContactID)
	assert.Equal(t, 0, historyTask.ErrorCount)

	err = tickets.HandleSendTicketHistory(ctx, rt, task)
	assert.NoError(t, err)
	assert.True(t, sendHistoryCalled)
}

func TestSendTicketHistoryRetry(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()
	rc := rp.Get()
	defer rc.Close()

	defer testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	mockTicketer := setupMockTicketer(t, db)
	modelTicket := createTestTicket(t, db, mockTicketer)

	sendHistoryCalled = false
	mockError = errors.New("temporary error")

	err := tickets.QueueSendHistory(rc, modelTicket, testdata.Cathy.ID)
	assert.NoError(t, err)

	// First attempt
	task, err := queue.PopNextTask(rc, queue.HandlerQueue)
	require.NoError(t, err)
	require.NotNil(t, task)

	err = tickets.HandleSendTicketHistory(ctx, rt, task)
	assert.NoError(t, err) // No error returned because task was requeued
	assert.True(t, sendHistoryCalled)

	// Should have been requeued
	task, err = queue.PopNextTask(rc, queue.HandlerQueue)
	require.NoError(t, err)
	require.NotNil(t, task)

	var historyTask tickets.SendHistoryTask
	err = json.Unmarshal(task.Task, &historyTask)
	assert.NoError(t, err)
	assert.Equal(t, 1, historyTask.ErrorCount)

	// Second attempt
	sendHistoryCalled = false
	err = tickets.HandleSendTicketHistory(ctx, rt, task)
	assert.NoError(t, err)
	assert.True(t, sendHistoryCalled)

	// Should have been requeued again
	task, err = queue.PopNextTask(rc, queue.HandlerQueue)
	require.NoError(t, err)
	require.NotNil(t, task)

	err = json.Unmarshal(task.Task, &historyTask)
	assert.NoError(t, err)
	assert.Equal(t, 2, historyTask.ErrorCount)

	// Third and final attempt
	sendHistoryCalled = false
	err = tickets.HandleSendTicketHistory(ctx, rt, task)
	assert.NoError(t, err)
	assert.True(t, sendHistoryCalled)

	// Should not be requeued (max retries reached)
	task, err = queue.PopNextTask(rc, queue.HandlerQueue)
	assert.NoError(t, err)
	assert.Nil(t, task)
}

func setupMockTicketer(t *testing.T, db *sqlx.DB) *testdata.Ticketer {
	models.RegisterTicketService("mock", func(cfg *runtime.Config, httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
		return &mockTicketService{}, nil
	})

	db.MustExec(`INSERT INTO tickets_ticketer (id, uuid, org_id, ticketer_type, name, config, created_on, modified_on, is_active, created_by_id, modified_by_id) 
	VALUES ($1, $2, $3, 'mock', 'Mock Ticketer', '{}', NOW(), NOW(), TRUE, 1, 1)`,
		9999, "550e8400-e29b-41d4-a716-446655440000", testdata.Org1.ID)

	return &testdata.Ticketer{9999, "550e8400-e29b-41d4-a716-446655440000"}
}

func createTestTicket(t *testing.T, db *sqlx.DB, mockTicketer *testdata.Ticketer) *models.Ticket {
	ticket := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, mockTicketer, testdata.DefaultTopic, "", "", nil)
	return ticket.Load(db)
}

// mockTicketService is a mock implementation of models.TicketService for testing
type mockTicketService struct {
}

var sendHistoryCalled bool
var mockError error

func (m *mockTicketService) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	sendHistoryCalled = true
	return mockError
}

func (m *mockTicketService) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (m *mockTicketService) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (m *mockTicketService) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID("550e8400-e29b-41d4-a716-446655440000"), "Mock Ticketer", "mock"))
	return flows.OpenTicket(ticketer, topic, body, assignee), nil
}

func (m *mockTicketService) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, attachments []utils.Attachment, metadata json.RawMessage, msgExternalID null.String, logHTTP flows.HTTPLogCallback) error {
	return nil
}
