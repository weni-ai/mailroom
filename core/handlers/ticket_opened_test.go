package handlers_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/actions"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/sqs"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/null"

	_ "github.com/nyaruka/mailroom/services/tickets/mailgun"
	_ "github.com/nyaruka/mailroom/services/tickets/zendesk"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sqsLogTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

func TestTicketOpened(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)
	testsuite.Reset(testsuite.ResetRedis)

	hook := logtest.NewGlobal()
	defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.mailgun.net/v3/tickets.rapidpro.io/messages": {
			httpx.NewMockResponse(200, nil, `{
				"id": "<20200426161758.1.590432020254B2BF@tickets.rapidpro.io>",
				"message": "Queued. Thank you."
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/users/search.json?external_id=b699a406-7e44-49be-9f01-1a82893e8a10": {
			httpx.NewMockResponse(200, nil, `{"users": []}`),
		},
		"https://nyaruka.zendesk.com/api/v2/users.json": {
			httpx.NewMockResponse(201, nil, `{
				"user": {
					"id": 9873843,
					"external_id": "b699a406-7e44-49be-9f01-1a82893e8a10",
					"name": "Bob",
					"role": "end-user"
				}
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/any_channel/push.json": {
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "123",
						"status": {"code": "success"}
					}
				]
			}`),
		},
	}))

	// an existing ticket
	cathyTicket := models.NewTicket(flows.TicketUUID(uuids.New()), testdata.Org1.ID, testdata.Cathy.ID, testdata.Mailgun.ID, "748363", testdata.DefaultTopic.ID, "Who?", models.NilUserID, nil)
	err := models.InsertTickets(ctx, db, []*models.Ticket{cathyTicket})
	require.NoError(t, err)

	tcs := []handlers.TestCase{
		{
			Actions: handlers.ContactActionMap{
				testdata.Cathy: []flows.Action{
					actions.NewOpenTicket(
						handlers.NewActionUUID(),
						assets.NewTicketerReference(testdata.Mailgun.UUID, "Mailgun (IT Support)"),
						assets.NewTopicReference(testdata.SupportTopic.UUID, "Support", ""),
						"Where are my cookies?",
						assets.NewUserReference(testdata.Admin.Email, "Admin"),
						"Email Ticket",
					),
				},
				testdata.Bob: []flows.Action{
					actions.NewOpenTicket(
						handlers.NewActionUUID(),
						assets.NewTicketerReference(testdata.Zendesk.UUID, "Zendesk (Nyaruka)"),
						nil,
						"I've found some cookies",
						nil,
						"Zen Ticket",
					),
				},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{ // cathy's old ticket will still be open and cathy's new ticket will have been created
					SQL:   "select count(*) from tickets_ticket where contact_id = $1 AND status = 'O' AND ticketer_id = $2",
					Args:  []interface{}{testdata.Cathy.ID, testdata.Mailgun.ID},
					Count: 2,
				},
				{ // and there's an HTTP log for that
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1",
					Args:  []interface{}{testdata.Mailgun.ID},
					Count: 1,
				},
				{ // which doesn't include our API token
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1 AND request like '%sesame%'",
					Args:  []interface{}{testdata.Mailgun.ID},
					Count: 0,
				},
				{ // bob's ticket will have been created too
					SQL:   "select count(*) from tickets_ticket where contact_id = $1 AND status = 'O' AND ticketer_id = $2",
					Args:  []interface{}{testdata.Bob.ID, testdata.Zendesk.ID},
					Count: 1,
				},
				{ // and there's an HTTP log for that
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1",
					Args:  []interface{}{testdata.Zendesk.ID},
					Count: 1,
				},
				{ // which doesn't include our API token
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1 AND request like '%523562%'",
					Args:  []interface{}{testdata.Zendesk.ID},
					Count: 0,
				},
				{ // and we have 2 ticket opened events for the 2 tickets opened
					SQL:   "select count(*) from tickets_ticketevent where event_type = 'O'",
					Count: 2,
				},
				{ // both of our tickets have a topic (one without an explicit topic get's the default)
					SQL:   "select count(*) from tickets_ticket where topic_id is null",
					Count: 0,
				},
				{ // one of our tickets is assigned to admin
					SQL:   "select count(*) from tickets_ticket where assignee_id = $1",
					Args:  []interface{}{testdata.Admin.ID},
					Count: 1,
				},
				{ // admin will have a ticket assigned notification for the ticket directly assigned to them
					SQL:   "select count(*) from notifications_notification where user_id = $1 and notification_type = 'tickets:activity'",
					Args:  []interface{}{testdata.Admin.ID},
					Count: 1,
				},
				{ // all assignable users will have a ticket opened notification for the unassigned ticket
					SQL:   "select count(*) from notifications_notification where notification_type = 'tickets:opened'",
					Args:  nil,
					Count: 3,
				},
			},
		},
	}

	db.MustExec(`UPDATE orgs_org SET config = '{"is_multi_agents": true}' WHERE id = $1`, testdata.Org1.ID)
	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	require.True(t, oa.Org().ConfigBoolValue("is_multi_agents", false))

	handlers.RunTestCases(t, ctx, rt, tcs)

	// Q-01 (flow path): the two tickets of a multi-agent org were queued for SQS with origin "flow" and logged
	rc := rp.Get()
	defer rc.Close()

	members, err := redis.ByteSlices(rc.Do("zrange", "sqs_publish:1", 0, -1))
	require.NoError(t, err)
	require.Len(t, members, 2)

	corrIDs := map[string]bool{}
	for _, member := range members {
		task := &queue.Task{}
		require.NoError(t, json.Unmarshal(member, task))

		p := &sqs.PublishTask{}
		require.NoError(t, json.Unmarshal(task.Task, p))
		require.NotNil(t, p.Meta)
		assert.Equal(t, "flow", p.Meta.Origin)
		assert.NotEmpty(t, p.Meta.TicketUUID)
		assert.NotEmpty(t, p.Meta.CorrelationID)
		assert.False(t, p.Meta.CreatedOn.IsZero())
		corrIDs[p.Meta.CorrelationID] = true
	}

	var enqueued []*logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Data["event"] == "enqueued" {
			enqueued = append(enqueued, e)
		}
	}
	require.Len(t, enqueued, 2)

	for _, e := range enqueued {
		assert.Equal(t, logrus.InfoLevel, e.Level)
		assert.NotEmpty(t, e.Data["ticket_uuid"])
		assert.True(t, corrIDs[e.Data["correlation_id"].(string)])
		assert.Equal(t, int(testdata.Org1.ID), e.Data["org_id"])
		assert.NotEmpty(t, e.Data["queue_url"])
		assert.Equal(t, "flow", e.Data["origin"])
		for _, field := range []string{"created_on", "queued_on", "enqueued_at"} {
			assert.Regexp(t, sqsLogTimeRegex, e.Data[field], field)
		}
	}
}

type mockTicketService struct {
	sendHistoryCalled bool
	tickets           []*models.Ticket
	ticketer          *flows.Ticketer
	contactID         models.ContactID
	runs              []*models.FlowRun
}

func (m *mockTicketService) SendHistory(ticket *models.Ticket, contactID models.ContactID, runs []*models.FlowRun, logHTTP flows.HTTPLogCallback) error {
	m.sendHistoryCalled = true
	m.tickets = append(m.tickets, ticket)
	m.contactID = contactID
	m.runs = runs
	return nil
}

func (m *mockTicketService) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (m *mockTicketService) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	return nil
}

func (m *mockTicketService) Open(session flows.Session, topic *flows.Topic, body string, assignee *flows.User, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	return flows.OpenTicket(m.ticketer, topic, body, assignee), nil
}

func (m *mockTicketService) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, msgType string, attachments []utils.Attachment, body json.RawMessage, subject null.String, logHTTP flows.HTTPLogCallback) error {
	return nil
}
