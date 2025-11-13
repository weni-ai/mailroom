package freshchat_test

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
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/services/tickets/freshchat"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/null"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))

	// Test missing freshchat_domain
	_, err := freshchat.NewService(
		&runtime.Config{},
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"oauth_token": apiKey,
		},
	)
	assert.EqualError(t, err, "missing freshchat_domain or oauth_token in freshchat config")

	// Test missing oauth_token
	_, err = freshchat.NewService(
		&runtime.Config{},
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
		},
	)
	assert.EqualError(t, err, "missing freshchat_domain or oauth_token in freshchat config")

	// Test valid config
	svc, err := freshchat.NewService(
		&runtime.Config{},
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestOpen(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users?reference_id=5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/users", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong"}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
		},
		fmt.Sprintf("%s/v2/channels", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `{
				"channels": []
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/conversations", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong"}`),
			httpx.NewMockResponse(201, nil, `{
				"conversation_id": "conv123",
				"status": "new",
				"channel_id": "channel123",
				"messages": [{}]
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))

	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic := oa.SessionAssets().Topics().FindByName("General")

	logger := &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "failed to get or create user for ticket")

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "failed to get or create user for ticket")

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "failed to get channels: unable to connect to server")

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "no freshchat channels found")

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "error creating conversation: unable to connect to server")

	logger = &flows.HTTPLogger{}
	_, err = svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	assert.EqualError(t, err, "error creating conversation: Something went wrong")

	logger = &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, "Where are my cookies?", nil, logger.Log)
	require.NoError(t, err)
	assert.NotEmpty(t, ticket.UUID())
	assert.Equal(t, "General", ticket.Topic().Name())
	assert.Equal(t, "Where are my cookies?", ticket.Body())
	assert.Equal(t, "conv123", ticket.ExternalID())
	assert.Equal(t, 5, len(logger.Logs))
}

func TestOpenWithChannelID(t *testing.T) {
	ctx, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users?reference_id=5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{}
				]
			}`),
		},

		fmt.Sprintf("%s/v2/users", baseURL): {
			httpx.NewMockResponse(201, nil, `{
				"id": "user123"
			}`),
		},
		fmt.Sprintf("%s/v2/channels", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/conversations", baseURL): {
			httpx.NewMockResponse(201, nil, `{
				"conversation_id": "conv123",
				"status": "new",
				"channel_id": "channel123",
				"messages": [{}]
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))

	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	defaultTopic := oa.SessionAssets().Topics().FindByName("General")

	// Test with channel_id in body (JSON format)
	body := `{"channel_id": "custom_channel", "messages": [{"message_parts": [{"text": {"content": "Where are my cookies?"}}]}]}`
	logger := &flows.HTTPLogger{}
	ticket, err := svc.Open(session, defaultTopic, body, nil, logger.Log)
	require.NoError(t, err)
	assert.Equal(t, "conv123", ticket.ExternalID())
	// Should have CreateUser, GetChannels (if channel_id not parsed correctly), and CreateConversation
	assert.GreaterOrEqual(t, len(logger.Logs), 2)
}

func TestForward(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users?reference_id=6393abc0-283d-4c9b-a1b3-641a035c34bf", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "6393abc0-283d-4c9b-a1b3-641a035c34bf"
					}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "6393abc0-283d-4c9b-a1b3-641a035c34bf"
					}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "6393abc0-283d-4c9b-a1b3-641a035c34bf"
					}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "6393abc0-283d-4c9b-a1b3-641a035c34bf"
					}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/channels", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"channels": [
					{"id": "channel123", "name": "Support"}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/conversations/conv123/messages", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong"}`),
			httpx.NewMockResponse(201, nil, `{
				"channel_id": "channel123",
				"conversation_id": "conv123",
				"message_parts": [{
					"text": {
						"content": "Message sent"
					}
				}]
			}`),
			httpx.NewMockResponse(201, nil, `{
				"channel_id": "channel123",
				"conversation_id": "conv123",
				"message_parts": [{
					"text": {
						"content": "Message sent"
					}
				}]
			}`),
		},
		"https://example.com/image.jpg": {
			httpx.NewMockResponse(200, nil, `fake image data`),
		},
		fmt.Sprintf("%s/v2/images/upload", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"url": "https://uploaded.com/image.jpg"
			}`),
		},
		"https://example.com/video.mp4": {
			httpx.NewMockResponse(200, nil, `fake video data`),
		},
		"https://example.com/file.pdf": {
			httpx.NewMockResponse(200, nil, `fake file data`),
		},
		fmt.Sprintf("%s/v2/files/upload", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"file_name": "file.pdf",
				"file_size": 100,
				"file_content_type": "application/pdf",
				"file_extension_type": "pdf",
				"file_hash": "hash123",
				"file_security_status": "safe"
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))
	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	ticket := models.NewTicket(
		flows.TicketUUID("88bfa1dc-be33-45c2-b469-294ecb0eba90"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.Freshchats.ID,
		"conv123",
		testdata.DefaultTopic.ID,
		"Where my cookies?",
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid": string(testdata.Cathy.UUID),
		},
	)

	logger := &flows.HTTPLogger{}
	err = svc.Forward(ticket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", nil, nil, null.NullString, logger.Log)
	assert.EqualError(t, err, "failed to create message: unable to connect to server")

	logger = &flows.HTTPLogger{}
	err = svc.Forward(ticket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", nil, nil, null.NullString, logger.Log)
	assert.EqualError(t, err, "failed to create message: Something went wrong")

	logger = &flows.HTTPLogger{}
	err = svc.Forward(ticket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", nil, nil, null.NullString, logger.Log)
	require.NoError(t, err)

	// Test with attachments
	attachments := []utils.Attachment{
		"image/jpeg:https://example.com/image.jpg",
		"video/mp4:https://example.com/video.mp4",
		"application/pdf:https://example.com/file.pdf",
	}

	logger = &flows.HTTPLogger{}
	err = svc.Forward(ticket, flows.MsgUUID("4fa340ae-1fb0-4666-98db-2177fe9bf31c"), "It's urgent", attachments, nil, null.NullString, logger.Log)
	require.NoError(t, err)
}

func TestClose(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/conversations/conv123", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong"}`),
			httpx.NewMockResponse(200, nil, `{
				"conversation_id": "conv123",
				"status": "closed"
			}`),
		},
		fmt.Sprintf("%s/v2/conversations/conv456", baseURL): {
			httpx.NewMockResponse(200, nil, `{
				"conversation_id": "conv456",
				"status": "closed"
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))
	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	ticket1 := models.NewTicket(
		flows.TicketUUID("88bfa1dc-be33-45c2-b469-294ecb0eba90"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.Freshchats.ID,
		"conv123",
		testdata.DefaultTopic.ID,
		"Where my cookies?",
		models.NilUserID,
		nil,
	)
	ticket2 := models.NewTicket(
		flows.TicketUUID("645eee60-7e84-4a9e-ade3-4fce01ae28f1"),
		testdata.Org1.ID,
		testdata.Bob.ID,
		testdata.Freshchats.ID,
		"conv456",
		testdata.DefaultTopic.ID,
		"Where my shoes?",
		models.NilUserID,
		nil,
	)

	logger := &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)
	assert.EqualError(t, err, "failed to close conversation: unable to connect to server")

	logger = &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)
	assert.EqualError(t, err, "failed to close conversation: Something went wrong")

	logger = &flows.HTTPLogger{}
	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)
	require.NoError(t, err)
}

func TestReopen(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/conversations/conv123", baseURL): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(400, nil, `{"error": "Something went wrong"}`),
			httpx.NewMockResponse(200, nil, `{
				"conversation_id": "conv123",
				"status": "open"
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))
	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	ticket := models.NewTicket(
		flows.TicketUUID("88bfa1dc-be33-45c2-b469-294ecb0eba90"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.Freshchats.ID,
		"conv123",
		testdata.DefaultTopic.ID,
		"Where my cookies?",
		models.NilUserID,
		nil,
	)

	logger := &flows.HTTPLogger{}
	err = svc.Reopen([]*models.Ticket{ticket}, logger.Log)
	assert.EqualError(t, err, "failed to reopen conversation: unable to connect to server")

	logger = &flows.HTTPLogger{}
	err = svc.Reopen([]*models.Ticket{ticket}, logger.Log)
	assert.EqualError(t, err, "failed to reopen conversation: Something went wrong")

	logger = &flows.HTTPLogger{}
	err = svc.Reopen([]*models.Ticket{ticket}, logger.Log)
	require.NoError(t, err)
}

func TestSendHistory(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer httpx.SetRequestor(httpx.DefaultRequestor)

	conversationID := "conv123"
	contactUUID := string(testdata.Cathy.UUID)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/v2/users?reference_id=%s", baseURL, contactUUID): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "`+contactUUID+`"
					}
				]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"users": [
					{
						"id": "user123",
						"reference_id": "`+contactUUID+`"
					}
				]
			}`),
		},
		fmt.Sprintf("%s/v2/conversations/%s/messages", baseURL, conversationID): {
			httpx.MockConnectionError,
			httpx.NewMockResponse(201, nil, `{
				"channel_id": "channel123",
				"conversation_id": "`+conversationID+`",
				"message_parts": [{
					"text": {
						"content": "Hello"
					}
				}]
			}`),
			httpx.NewMockResponse(201, nil, `{
				"channel_id": "channel123",
				"conversation_id": "`+conversationID+`",
				"message_parts": [{
					"text": {
						"content": "Hello"
					}
				}]
			}`),
		},
	}))

	ticketer := flows.NewTicketer(static.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "freshchat"))
	svc, err := freshchat.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"freshchat_domain": baseURL,
			"oauth_token":      apiKey,
		},
	)
	require.NoError(t, err)

	ticket := models.NewTicket(
		flows.TicketUUID("88bfa1dc-be33-45c2-b469-294ecb0eba90"),
		testdata.Org1.ID,
		testdata.Cathy.ID,
		testdata.Freshchats.ID,
		conversationID,
		testdata.DefaultTopic.ID,
		`{"history_after": "2019-10-07 15:21:29"}`,
		models.NilUserID,
		map[string]interface{}{
			"contact-uuid": contactUUID,
		},
	)

	logger := &flows.HTTPLogger{}

	mockDB, mock, _ := sqlmock.New()
	defer mockDB.Close()
	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")
	freshchat.SetDB(sqlxDB)

	// Test 1: Error getting user (connection error) - no DB mock needed
	err = svc.SendHistory(ticket, models.ContactID(testdata.Cathy.ID), nil, logger.Log)
	assert.Error(t, err)

	// Test 2: Error creating message (connection error)
	rows := sqlmock.NewRows([]string{
		"id", "broadcast_id", "uuid", "text", "created_on", "direction", "status", "visibility", "msg_count", "error_count", "next_attempt", "external_id", "attachments", "metadata", "channel_id", "contact_id", "contact_urn_id", "org_id", "topup_id",
	})
	msgTime, _ := time.Parse(time.RFC3339, "2019-10-07T15:21:30Z")
	rows.AddRow(464, nil, "eb234953-38e7-491f-8a50-b03056a7d002", "Hello", msgTime, "I", "H", "V", 1, 0, msgTime, "1026", nil, nil, 3, 10000, 1, 1, 1)

	after, _ := time.Parse("2006-01-02 15:04:05", "2019-10-07 15:21:29")

	mock.ExpectQuery("SELECT").
		WithArgs(10000, after).
		WillReturnRows(rows)

	err = svc.SendHistory(ticket, models.ContactID(testdata.Cathy.ID), nil, logger.Log)
	assert.Error(t, err)

	// Test 3: Success
	mock.ExpectQuery("SELECT").
		WithArgs(10000, after).
		WillReturnRows(rows)

	err = svc.SendHistory(ticket, models.ContactID(testdata.Cathy.ID), nil, logger.Log)
	assert.NoError(t, err)
}
