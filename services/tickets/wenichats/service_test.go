package wenichats_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
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
	"github.com/nyaruka/mailroom/services/tickets/wenichats"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAndForward(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	testsuite.Reset(testsuite.ResetData | testsuite.ResetStorage)

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/rooms/", baseURL): {
			httpx.NewMockResponse(201, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry",
					"age": 23,
					"join_date": "2017-12-02",
					"gender": "male"
				},
				"callback_url": "http://example.com"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"uuid": "e5cbc781-4e0e-4954-b078-0373308e11c3",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry",
					"age": 23,
					"join_date": "2017-12-02",
					"gender": "male"
				},
				"callback_url": "http://example.com"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"uuid": "9688d21d-95aa-4bed-afc7-f31b35731a3d",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry",
					"age": 23,
					"join_date": "2017-12-02",
					"gender": "male"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/rooms/8ecb1e4a-b457-4645-a161-e2b02ddffa88/", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/rooms/e5cbc781-4e0e-4954-b078-0373308e11c3/", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"uuid": "e5cbc781-4e0e-4954-b078-0373308e11c3",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/rooms/9688d21d-95aa-4bed-afc7-f31b35731a3d/", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"uuid": "9688d21d-95aa-4bed-afc7-f31b35731a3d",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/msgs/", baseURL): {
			// httpx.NewMockResponse(200, nil, `{}`),
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `{
				"uuid": "b9312612-c26d-45ec-b9bb-7f116771fdd6",
				"user": null,
				"room": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"contact": {
					"uuid": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"text": "Where are my cookies?",
				"seen": false,
				"media": [
					{
						"content_type": "audio/wav",
						"url": "http://domain.com/recording.wav"
					}
				],
				"created_on": "2022-08-25T02:06:55.885000-03:00"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"uuid": "b9312612-c26d-45ec-b9bb-7f116771fdd6",
				"user": null,
				"room": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"contact": {
					"uuid": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"text": "Where are my cookies?",
				"seen": false,
				"media": [
					{
						"content_type": "image/jpg",
						"url": "https://link.to/dummy_image.jpg"
					},
					{
						"content_type": "video/mp4",
						"url": "https://link.to/dummy_video.mp4"
					},
					{
						"content_type": "audio/ogg",
						"url": "https://link.to/dummy_audio.ogg"
					}
				],
				"created_on": "2022-08-25T02:06:55.885000-03:00"
			}`),
		},
		"https://link.to/dummy_image.jpg": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "image/jpeg"}, `imagebytes`),
		},
		"https://link.to/dummy_video.mp4": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "video/mp4"}, `videobytes`),
		},
		"https://link.to/dummy_audio.ogg": {
			httpx.NewMockResponse(200, map[string]string{"Content-Type": "audio/ogg"}, `audiobytes`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "wenichats"))

	_, err = wenichats.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{},
	)
	assert.EqualError(t, err, "missing project_auth or sector_uuid")

	mockDB, mock, _ := sqlmock.New()
	defer mockDB.Close()
	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	rows := sqlmock.NewRows([]string{
		"id", "broadcast_id", "uuid", "text", "created_on", "direction", "status", "visibility", "msg_count", "error_count", "next_attempt", "external_id", "attachments", "metadata", "channel_id", "contact_id", "contact_urn_id", "org_id", "topup_id",
	})
	msgTime, _ := time.Parse(time.RFC3339, "2019-10-07T15:21:30Z")
	rows.AddRow(464, nil, "eb234953-38e7-491f-8a50-b03056a7d002", "ahoy", msgTime, "I", "H", "V", 1, 0, msgTime, "1026", nil, nil, 3, 1234567, 1, 1, 1)

	mock.ExpectQuery("SELECT").WithArgs("1ae96956-4b34-433e-8d1a-f05fe6923d6d").WillReturnRows(
		sqlmock.NewRows([]string{"row_to_json"}).AddRow(`{"uuid": "1ae96956-4b34-433e-8d1a-f05fe6923d6d", "id": 1, "name": "WeniChats", "ticketer_type": "wenichats", "config": {"project_uuid": "8a4bae05-993c-4f3b-91b5-80f4e09951f2", "project_name_origin": "Project 1"}}`),
	)

	after, err := time.Parse("2006-01-02T15:04:05", "2019-10-07T15:21:29")
	assert.NoError(t, err)

	mock.ExpectQuery("SELECT").
		WithArgs(1234567, after).
		WillReturnRows(rows)

	wenichats.SetDB(sqlxDB)

	svc, err := wenichats.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"project_auth": authToken,
			"sector_uuid":  "1a4bae05-993c-4f3b-91b5-80f4e09951f2",
		},
	)
	assert.NoError(t, err)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic := oa.SessionAssets().Topics().FindByName("General")

	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, `{"custom_fields":{"country": "brazil","mood": "angry"}}`, nil, logger.Log)

	assert.NoError(t, err)
	assert.Equal(t, flows.TicketUUID("e7187099-7d38-4f60-955c-325957214c42"), ticket.UUID())
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, `{"custom_fields":{"country": "brazil","mood": "angry"}}`, ticket.Body())
	assert.Equal(t, "8ecb1e4a-b457-4645-a161-e2b02ddffa88", ticket.ExternalID())
	assert.Equal(t, 1, len(logger.Logs))
	test.AssertSnapshot(t, "open_ticket", logger.Logs[0].Request)

	dbTicket := models.NewTicket(ticket.UUID(), testdata.Org1.ID, testdata.Cathy.ID, testdata.Wenichats.ID, "8ecb1e4a-b457-4645-a161-e2b02ddffa88", testdata.DefaultTopic.ID, "Where are my cookies?", models.NilUserID, map[string]interface{}{
		"contact-uuid":    string(testdata.Cathy.UUID),
		"contact-display": "Cathy",
	})
	logger = &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", nil, logger.Log)
	assert.EqualError(t, err, "error send message to wenichats: unable to connect to server")

	logger = &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", nil, logger.Log)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(logger.Logs))
	test.AssertSnapshot(t, "forward_message", logger.Logs[0].Request)

	dbTicket2 := models.NewTicket("645eee60-7e84-4a9e-ade3-4fce01ae28f1", testdata.Org1.ID, testdata.Cathy.ID, testdata.Wenichats.ID, "8ecb1e4a-b457-4645-a161-e2b02ddffa88", testdata.DefaultTopic.ID, "Where are my cookies?", models.NilUserID, map[string]interface{}{
		"contact-uuid":    string(testdata.Cathy.UUID),
		"contact-display": "Cathy",
	})

	logger = &flows.HTTPLogger{}
	attachments := []utils.Attachment{
		"image/jpg:https://link.to/dummy_image.jpg",
		"video/mp4:https://link.to/dummy_video.mp4",
		"audio/ogg:https://link.to/dummy_audio.ogg",
	}
	err = svc.Forward(dbTicket2, flows.MsgUUID("5ga340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", attachments, logger.Log)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(logger.Logs))

	// test open with body empty
	logger2 := &flows.HTTPLogger{}

	wenichats.SetDB(rt.DB)

	oa, err = models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic = oa.SessionAssets().Topics().FindByName("General")

	ticket, err = svc.Open(session, defaultTopic, "", nil, logger2.Log)

	assert.NoError(t, err)
	assert.Equal(t, flows.TicketUUID("59d74b86-3e2f-4a93-aece-b05d2fdcde0c"), ticket.UUID())
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, "", ticket.Body())
	assert.Equal(t, "e5cbc781-4e0e-4954-b078-0373308e11c3", ticket.ExternalID())
	assert.Equal(t, 1, len(logger2.Logs))
	test.AssertSnapshot(t, "open_ticket_empty_body", logger2.Logs[0].Request)

	//test with history after option
	logger3 := &flows.HTTPLogger{}

	wenichats.SetDB(rt.DB)
	oa, err = models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic = oa.SessionAssets().Topics().FindByName("General")

	ticket, err = svc.Open(session, defaultTopic, "{\"history_after\":\"2019-10-07 15:21:30\"}", nil, logger3.Log)

	assert.NoError(t, err)
	assert.Equal(t, flows.TicketUUID("9688d21d-95aa-4bed-afc7-f31b35731a3d"), ticket.UUID())
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, "{\"history_after\":\"2019-10-07 15:21:30\"}", ticket.Body())
	assert.Equal(t, "9688d21d-95aa-4bed-afc7-f31b35731a3d", ticket.ExternalID())
	assert.Equal(t, 1, len(logger3.Logs))
	test.AssertSnapshot(t, "open_ticket_history_after", logger3.Logs[0].Request)

	session.Contact().ClearURNs()
	_, err = svc.Open(session, defaultTopic, "{\"history_after\":\"2019-10-07 15:21:30\"}", nil, logger3.Log)

	assert.Equal(t, "failed to open ticket, no urn found for contact", err.Error())
}

func TestOpenFails(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()
	testsuite.Reset(testsuite.ResetData | testsuite.ResetStorage)

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/rooms/", baseURL): {
			httpx.NewMockResponse(502, nil, `502 Bad Gateway`),
			httpx.NewMockResponse(201, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry",
					"age": 23,
					"join_date": "2017-12-02",
					"gender": "male"
				},
				"callback_url": "http://example.com"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry",
					"age": 23,
					"join_date": "2017-12-02",
					"gender": "male"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/rooms/8ecb1e4a-b457-4645-a161-e2b02ddffa88/", baseURL): {
			httpx.NewMockResponse(502, nil, `502 Bad Gateway`),
			httpx.NewMockResponse(200, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
		},
		fmt.Sprintf("%s/rooms/8ecb1e4a-b457-4645-a161-e2b02ddffa88/close/", baseURL): {
			httpx.NewMockResponse(200, nil, `{}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "wenichats"))

	svc, err := wenichats.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"project_auth": authToken,
			"sector_uuid":  "1a4bae05-993c-4f3b-91b5-80f4e09951f2",
		},
	)
	assert.NoError(t, err)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic := oa.SessionAssets().Topics().FindByName("General")

	// Error on Create Room
	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, `{"custom_fields":{"country": "brazil","mood": "angry"}}`, nil, logger.Log)

	assert.Nil(t, ticket)
	assert.Error(t, err)

	// Error on SelectHistory like a timeout
	mockDB, _, _ := sqlmock.New()
	defer mockDB.Close()
	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	wenichats.SetDB(sqlxDB)

	logger = &flows.HTTPLogger{}
	ticket, err = svc.Open(session, defaultTopic, `{"custom_fields":{"country": "brazil","mood": "angry"}}`, nil, logger.Log)

	assert.Nil(t, ticket)
	assert.Error(t, err)

}

func TestCloseAndReopen(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	roomUUID := "8ecb1e4a-b457-4645-a161-e2b02ddffa88"

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/rooms/%s/close/", baseURL, roomUUID): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(500, nil, `{
				"message": "error"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"uuid": "8ecb1e4a-b457-4645-a161-e2b02ddffa88",
				"user": {
					"first_name": "John",
					"last_name": "Doe",
					"email": "john.doe@chats.weni.ai"
				},
				"contact": {
					"external_id": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f",
					"name": "Foo Bar",
					"email": "FooBar@weni.ai",
					"status": "string",
					"phone": "+250788123123",
					"custom_fields": {},
					"created_on": "2019-08-24T14:15:22Z"
				},
				"queue": {
					"uuid": "449f48d9-4905-4d6f-8abf-f1ff6afb803e",
					"created_on": "2019-08-24T14:15:22Z",
					"modified_on": "2019-08-24T14:15:22Z",
					"name": "CHATS",
					"sector": "f3d496ff-c154-4a96-a678-6a8879583ddb"
				},
				"created_on": "2019-08-24T14:15:22Z",
				"modified_on": "2019-08-24T14:15:22Z",
				"is_active": true,
				"custom_fields": {
					"country": "brazil",
					"mood": "angry"
				},
				"callback_url": "http://example.com"
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "wenichats"))

	svc, err := wenichats.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"project_auth": authToken,
			"sector_uuid":  "1a4bae05-993c-4f3b-91b5-80f4e09951f2",
		},
	)
	assert.NoError(t, err)

	ticket1 := models.NewTicket("88bfa1dc-be33-45c2-b469-294ecb0eba90", testdata.Org1.ID, testdata.Cathy.ID, testdata.Wenichats.ID, roomUUID, testdata.DefaultTopic.ID, "Where my cookies?", models.NilUserID, nil)
	ticket2 := models.NewTicket("645eee60-7e84-4a9e-ade3-4fce01ae28f1", testdata.Org1.ID, testdata.Bob.ID, testdata.Wenichats.ID, roomUUID, testdata.DefaultTopic.ID, "Where my shoes?", models.NilUserID, nil)

	logger := &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)
	assert.NoError(t, err)

	logger = &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)
	assert.NoError(t, err)
	test.AssertSnapshot(t, "close_tickets", logger.Logs[0].Request)

	err = svc.Reopen([]*models.Ticket{ticket2}, logger.Log)
	assert.EqualError(t, err, "wenichats ticket type doesn't support reopening")
}
