package chatgpt_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/buger/jsonparser"
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
	testsuite.Reset(testsuite.ResetData | testsuite.ResetStorage)

	defer dates.SetNowSource(dates.DefaultNowSource)
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))

	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api/openai.com/v1/chat/completions": {
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

	svc, err := chatgpt.NewService(
		rt.Config,
		http.DefaultClient,
		nil,
		chatgptService,
		map[string]string{"api_key": apiKey},
	)
	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	callAction := assets.ExternalServiceCallAction{Name: "CreateCompletion", Value: "CreateCompletion"}
	params := []assets.ExternalServiceParam{}
	call, err := svc.Call(session, callAction, params, logger.Log)
	assert.EqualError(t, err, "error on call openai create completion: message:. type:invalid_request_error")
	assert.Nil(t, call)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api/openai.com/v1/chat/completions": {
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
		map[string]string{"api_key": apiKey},
	)

	callAction = assets.ExternalServiceCallAction{Name: "CreateCompletion", Value: "CreateCompletion"}
	params = []assets.ExternalServiceParam{}
	call, err = svc.Call(session, callAction, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	msgContent, err := jsonparser.GetString(call.ResponseJSON, "choices", "[0]", "message", "content")
	assert.NoError(t, err)
	assert.Equal(t, "This is a test!", msgContent)
}
