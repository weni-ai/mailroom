package chatgpt_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/buger/jsonparser"
	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	"github.com/nyaruka/mailroom/services/external/openai/chatgpt"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCall(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.openai.com/v1/chat/completions": {
			httpx.NewMockResponse(400, nil, `{
				"error": {
						"message": "",
						"type": "invalid_request_error",
						"param": null,
						"code": "invalid_api_key"
				}
		}`),
		},
	}))

	chatgptService := flows.NewExternalService(static.NewExternalService(assets.ExternalServiceUUID(uuids.New()), "chatgpt", "chatgpt"))

	_, err = chatgpt.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		chatgptService,
		map[string]string{},
	)
	assert.EqualError(t, err, "missing api_key in external service for chatgpt config")

	rt.Config.DB = "postgres://mailroom_test:temba@localhost/mailroom_test?sslmode=disable&Timezone=UTC"
	svc, err := chatgpt.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		chatgptService,
		map[string]string{"api_key": apiKey},
	)
	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	callAction := assets.ExternalServiceCallAction{Name: "ConsultarChatGPT", Value: "ConsultarChatGPT"}
	params := []assets.ExternalServiceParam{}
	call, err := svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call openai create completion: message:. type:invalid_request_error")
	assert.Nil(t, call)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.openai.com/v1/chat/completions": {
			httpx.NewMockResponse(200, nil, `{
				"id": "chatcmpl-7J0hfe9HOXQw5AsfC5jxylO5QRjpW",
				"object": "chat.completion",
				"created": 1684765291,
				"model": "gpt-3.5-turbo-0301",
				"usage": {
					"prompt_tokens": 14,
					"completion_tokens": 5,
					"total_tokens": 19
				},
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "This is a test!"
						},
						"finish_reason": "stop",
						"index": 0
					}
				]
			}`),
		},
	}))

	svc, err = chatgpt.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		chatgptService,
		map[string]string{
			"api_key":        apiKey,
			"rules":          "mock rules",
			"knowledge_base": "mock knowledge base",
		},
	)
	assert.NoError(t, err)

	callAction = assets.ExternalServiceCallAction{Name: "ConsultarChatGPT", Value: "ConsultarChatGPT"}
	params = []assets.ExternalServiceParam{}

	jsonParams := `[
		{"type": "AditionalPrompts", "data": {"value": [{"text": "prompt test"}]}},
		{"type": "SendCompleteHistory", "data": {"value": true}},
		{"type": "UserInput", "data": {"value": "Say This is a test!"}}
	]`
	err = json.Unmarshal([]byte(jsonParams), &params)
	assert.NoError(t, err)

	// set a mock db with message for test history
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()
	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	dummyTime, _ := time.Parse(time.RFC1123, "2019-10-07T15:21:30")

	rows := sqlmock.NewRows([]string{"id", "uuid", "text", "high_priority", "created_on", "modified_on", "sent_on", "queued_on", "direction", "status", "visibility", "msg_type", "msg_count", "error_count", "next_attempt", "external_id", "attachments", "metadata", "broadcast_id", "channel_id", "contact_id", "contact_urn_id", "org_id", "topup_id"}).
		AddRow(100, "1348d654-e3dc-4f2f-add0-a9163dc48895", "I have a request", true, dummyTime, dummyTime, dummyTime, dummyTime, "O", "W", "V", "F", 1, 0, nil, "398", nil, nil, nil, 3, 1234567, 2, 3, 3)

	after, err := time.Parse("2006-01-02T15:04:05", "2019-10-07T15:21:30")
	assert.NoError(t, err)

	mock.ExpectQuery("SELECT").
		WithArgs(1234567, after).
		WillReturnRows(rows)

	chatgpt.SetDB(sqlxDB)

	// call external service action
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	msgContent, err := jsonparser.GetString(call.ResponseJSON, "choices", "[0]", "message", "content")
	assert.NoError(t, err)
	assert.Equal(t, "This is a test!", msgContent)
}
