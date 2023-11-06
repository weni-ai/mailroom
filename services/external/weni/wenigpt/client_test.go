package wenigpt_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/external/weni/wenigpt"
	"github.com/stretchr/testify/assert"
)

const (
	baseURL       = "https://wenigpt.weni.ai"
	authorization = "098e5a87-7221-45ba-9f06-98d066fed8e5"
	cookie        = "4f01f95e-fe65-4484-92d6-d7bff41fa06e"
)

func TestRequest(t *testing.T) {
	client := wenigpt.NewClient(http.DefaultClient, nil, baseURL, authorization, cookie)

	_, err := client.Request("POST", "", func() {}, nil)
	assert.Error(t, err)

	_, err = client.Request("{[:INVALID:]}", "", nil, nil)
	assert.Error(t, err)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		baseURL: {
			httpx.NewMockResponse(400, nil, `{
				"error":  "dummy error message"
			}`),
			httpx.NewMockResponse(200, nil, `{}`),
			httpx.NewMockResponse(400, nil, `{
				"error": "dummy error message"
			}`),
		},
	}))

	_, err = client.Request("POST", baseURL, nil, nil)
	assert.Error(t, err)

	_, err = client.Request("POST", baseURL, nil, nil)
	assert.Nil(t, err)

	response := new(interface{})
	_, err = client.Request("POST", baseURL, nil, response)
	assert.Error(t, err)
}

func TestWeniGPTRequest(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		baseURL: {
			httpx.NewMockResponse(400, nil, `{
				"error": "dummy error message"
			}`),
			httpx.NewMockResponse(200, nil, `{
				"delayTime": 2,
				"executionTime": 2,
				"id": "66b6a02c-b6e5-4e94-be8b-c631875b24d1",
				"status": "COMPLETED",
				"output": {
					"text": ["banana"]
				}
			}`),
		},
	}))

	client := wenigpt.NewClient(http.DefaultClient, nil, baseURL, authorization, cookie)

	data := wenigpt.NewWenigptRequest("Request: say wenigpt response output text. Response", 0, 0.0, 0.0, true, wenigpt.DefaultStopSequences)

	_, _, err := client.WeniGPTRequest(nil)
	assert.EqualError(t, err, "dummy error message")

	wmsg, _, err := client.WeniGPTRequest(data)
	assert.NoError(t, err)
	assert.Equal(t, "banana", wmsg.Output.Text[0])
}
