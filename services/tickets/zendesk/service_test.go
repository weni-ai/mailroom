package zendesk_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/presign"
	"github.com/nyaruka/mailroom/services/tickets/zendesk"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/null"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fieldTicket  = `{"message":"Cookies","priority":"high","subject":"Where are my cookies?","description":"I want to know where is my cookie.","tags": ["TAG_01","TAG_02"],"custom_fields":[{"id":"21938362","value":"hd_3000"}]}`
	fieldTicket1 = `{"id":10}`
)

func TestOpenAndForward(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer dates.SetNowSource(dates.DefaultNowSource)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		// "https://nyaruka.zendesk.com/api/v2/users/search.json?external_id=5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f": {
		// 	httpx.NewMockResponse(200, nil, `{"users": [{"id": 35241, "name": "Dummy User"}], "count": 1, "next_page": "https://nyaruka.zendesk.com/api/v2/users.json?page=2"}`),
		// 	httpx.NewMockResponse(200, nil, `{"users": [{"id": 35241, "name": "Dummy User"}], "count": 1, "next_page": "https://nyaruka.zendesk.com/api/v2/users.json?page=2"}`),
		// 	httpx.NewMockResponse(200, nil, `{"users": [{"id": 35241, "name": "Dummy User"}], "count": 1, "next_page": "https://nyaruka.zendesk.com/api/v2/users.json?page=2"}`),
		// },
		"https://nyaruka.zendesk.com/api/v2/any_channel/push.json": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "123",
						"status": {"code": "success"}
					}
				]
			}`),
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "124",
						"status": {"code": "success"}
					}
				]
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/tickets?external_id=59d74b86-3e2f-4a93-aece-b05d2fdcde0c": {
			httpx.NewMockResponse(200, nil, `{
				"tickets": [
				{
					"id": 1234,
					"subject": "Where are my cookie?"
				}
			]}`),
		},
		"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=10": {
			httpx.NewMockResponse(200, nil, `{
				"job_status": {
					"id": "1234-abcd",
					"url": "http://zendesk.com",
					"status": "queued"
				}}`),
		},
	}))

	tickUUID := assets.TicketerUUID(uuids.New())
	flowTicketer := flows.NewTicketer(static.NewTicketer(tickUUID, "Support", "zendesk"))

	_, err = zendesk.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		flowTicketer,
		models.BuildTicketer(models.TicketerID(0), tickUUID, testdata.Org1.ID, "zendesk", "Support", map[string]string{}),
		ctx,
		nil,
	)
	assert.EqualError(t, err, "missing subdomain or secret or oauth_token or push_id or push_token in zendesk config")

	svc, err := zendesk.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		flowTicketer,
		models.BuildTicketer(models.TicketerID(0), tickUUID, testdata.Org1.ID, "zendesk", "Support", map[string]string{
			"subdomain":   "nyaruka",
			"secret":      "sesame",
			"oauth_token": "987654321",
			"push_id":     "1234-abcd",
			"push_token":  "123456789",
		}),
		ctx,
		nil,
	)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic := oa.SessionAssets().Topics().FindByName("General")

	// try with connection failure
	_, err = svc.Open(session, defaultTopic, fieldTicket, nil, logger.Log)
	assert.EqualError(t, err, "error pushing message to zendesk: unable to connect to server")

	logger = &flows.HTTPLogger{}

	ticket, err := svc.Open(session, defaultTopic, fieldTicket, nil, logger.Log)
	assert.NoError(t, err)
	assert.Equal(t, flows.TicketUUID("59d74b86-3e2f-4a93-aece-b05d2fdcde0c"), ticket.UUID())
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, fieldTicket, ticket.Body())
	assert.Equal(t, "", ticket.ExternalID())
	assert.Equal(t, 1, len(logger.Logs))
	test.AssertSnapshot(t, "open_ticket", logger.Logs[0].Request)

	dbTicket := models.NewTicket(ticket.UUID(), testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "", testdata.DefaultTopic.ID, "Where are my cookies?", models.NilUserID, map[string]interface{}{
		"contact-uuid":    string(testdata.Cathy.UUID),
		"contact-display": "Cathy",
	})

	logger = &flows.HTTPLogger{}
	err = svc.Forward(
		dbTicket,
		flows.MsgUUID("ca5607f0-cba8-4c94-9cd5-c4fbc24aa767"),
		"It's urgent",
		[]utils.Attachment{utils.Attachment("image/jpg:http://myfiles.com/media/0123/attachment1.jpg")},
		nil,
		null.NullString,
		logger.Log,
	)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(logger.Logs))
	test.AssertSnapshot(t, "forward_message", logger.Logs[0].Request)

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, fieldTicket1, nil, logger.Log)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(logger.Logs))
	test.AssertSnapshot(t, "open", logger.Logs[0].Request)
}

const bucketPhoto = "https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg"

func TestOpenRewritesBucketMediaLinks(t *testing.T) {
	testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
	body := "history file " + bucketPhoto
	svc, session, topic := newZendeskOpen(t, pushOK())

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, topic, body, nil, logger.Log)
	require.NoError(t, err)
	require.Len(t, logger.Logs, 1)

	durable := presign.DurableURL(bucketPhoto)
	assert.Contains(t, logger.Logs[0].Request, durable)
	assert.NotContains(t, logger.Logs[0].Request, "media-bucket.s3.")
	assert.Equal(t, body, ticket.Body())
}

func TestOpenRewritesJSONFields(t *testing.T) {
	testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
	public := "https://example.com/a.jpg"
	body := `{"message":"hist ` + bucketPhoto + `","priority":"high","subject":"See ` + public + `","description":"file ` + bucketPhoto + `","custom_fields":[{"id":"21938362","value":"cf ` + bucketPhoto + `"}]}`
	svc, session, topic := newZendeskOpen(t, pushOK())

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, topic, body, nil, logger.Log)
	require.NoError(t, err)
	require.Len(t, logger.Logs, 1)

	request := logger.Logs[0].Request
	assert.Contains(t, request, presign.DurableURL(bucketPhoto))
	assert.Contains(t, request, public)
	assert.NotContains(t, request, "media-bucket.s3.")
	assert.Contains(t, request, `"id":"message"`)
	assert.Contains(t, request, `"id":"description"`)
	assert.Contains(t, request, `"id":"21938362"`)
	assert.Equal(t, body, ticket.Body())
}

func TestOpenFallbackAndReopen(t *testing.T) {
	body := "history file " + bucketPhoto

	t.Run("signed fallback", func(t *testing.T) {
		testsuite.UseAttachmentSigner(t, "media-bucket")
		presign.ClearLinkConfig()
		svc, session, topic := newZendeskOpen(t, pushOK())

		logger := &flows.HTTPLogger{}
		ticket, err := svc.Open(session, topic, body, nil, logger.Log)
		require.NoError(t, err)
		require.NotNil(t, ticket)
		require.Len(t, logger.Logs, 1)
		assert.Contains(t, logger.Logs[0].Request, "X-Amz-Algorithm")
	})

	t.Run("original when nothing can sign", func(t *testing.T) {
		presign.SetSigner(nil, 0)
		presign.ClearLinkConfig()
		svc, session, topic := newZendeskOpen(t, pushOK())

		logger := &flows.HTTPLogger{}
		ticket, err := svc.Open(session, topic, body, nil, logger.Log)
		require.NoError(t, err)
		require.NotNil(t, ticket)
		require.Len(t, logger.Logs, 1)
		assert.Contains(t, logger.Logs[0].Request, bucketPhoto)
		assert.NotContains(t, logger.Logs[0].Request, "X-Amz-Algorithm")
	})

	t.Run("reopen by id does not push opening text", func(t *testing.T) {
		svc, session, topic := newZendeskOpen(t, map[string][]httpx.MockResponse{
			"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=10": {
				httpx.NewMockResponse(200, nil, `{"job_status":{"id":"1234-abcd","url":"http://zendesk.com","status":"queued"}}`),
			},
		})

		logger := &flows.HTTPLogger{}
		_, err := svc.Open(session, topic, fieldTicket1, nil, logger.Log)
		require.NoError(t, err)
		require.Len(t, logger.Logs, 1)
		assert.Contains(t, logger.Logs[0].Request, "update_many.json")
		assert.NotContains(t, logger.Logs[0].Request, "external_resources")
		assert.NotContains(t, logger.Logs[0].Request, bucketPhoto)
	})
}

func pushOK() map[string][]httpx.MockResponse {
	return map[string][]httpx.MockResponse{
		"https://nyaruka.zendesk.com/api/v2/any_channel/push.json": {
			httpx.NewMockResponse(201, nil, `{"results":[{"external_resource_id":"123","status":{"code":"success"}}]}`),
		},
	}
}

func newZendeskOpen(t *testing.T, mocks map[string][]httpx.MockResponse) (models.TicketService, flows.Session, *flows.Topic) {
	t.Helper()
	ctx, rt, _, _ := testsuite.Get()
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
	t.Cleanup(func() { uuids.SetGenerator(uuids.DefaultGenerator) })
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))
	t.Cleanup(func() { dates.SetNowSource(dates.DefaultNowSource) })
	httpx.SetRequestor(httpx.NewMockRequestor(mocks))
	t.Cleanup(func() { httpx.SetRequestor(httpx.DefaultRequestor) })

	tickUUID := assets.TicketerUUID(uuids.New())
	flowTicketer := flows.NewTicketer(static.NewTicketer(tickUUID, "Support", "zendesk"))
	svc, err := zendesk.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		flowTicketer,
		models.BuildTicketer(models.TicketerID(0), tickUUID, testdata.Org1.ID, "zendesk", "Support", map[string]string{
			"subdomain":   "nyaruka",
			"secret":      "sesame",
			"oauth_token": "987654321",
			"push_id":     "1234-abcd",
			"push_token":  "123456789",
		}),
		ctx,
		nil,
	)
	require.NoError(t, err)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	return svc, session, oa.SessionAssets().Topics().FindByName("General")
}

func TestCloseAndReopen(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=12,14": {
			httpx.NewMockResponse(201, nil, `{
				"job_status": {
					"id": "1234-abcd",
					"url": "http://zendesk.com",
					"status": "queued"
				}
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=14": {
			httpx.NewMockResponse(201, nil, `{
				"job_status": {
					"id": "1234-abcd",
					"url": "http://zendesk.com",
					"status": "queued"
				}
			}`),
		},
	}))

	tickUUID := assets.TicketerUUID(uuids.New())
	flowTicketer := flows.NewTicketer(static.NewTicketer(tickUUID, "Support", "zendesk"))
	model := models.BuildTicketer(models.TicketerID(0), tickUUID, testdata.Org1.ID, "zendesk", "Support", map[string]string{
		"subdomain":   "nyaruka",
		"secret":      "sesame",
		"oauth_token": "987654321",
		"push_id":     "1234-abcd",
		"push_token":  "123456789",
	})
	svc, err := zendesk.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		flowTicketer,
		model,
		context.Background(),
		nil,
	)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	ticket1 := models.NewTicket("88bfa1dc-be33-45c2-b469-294ecb0eba90", testdata.Org1.ID, testdata.Cathy.ID, testdata.Zendesk.ID, "12", testdata.DefaultTopic.ID, "Where my cookies?", models.NilUserID, nil)
	ticket2 := models.NewTicket("645eee60-7e84-4a9e-ade3-4fce01ae28f1", testdata.Org1.ID, testdata.Bob.ID, testdata.Zendesk.ID, "14", testdata.DefaultTopic.ID, "Where my shoes?", models.NilUserID, nil)

	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)

	assert.NoError(t, err)
	test.AssertSnapshot(t, "close_tickets", logger.Logs[0].Request)

	err = svc.Reopen([]*models.Ticket{ticket2}, logger.Log)

	assert.NoError(t, err)
	test.AssertSnapshot(t, "reopen_tickets", logger.Logs[1].Request)
}
