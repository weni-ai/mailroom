package sentenx_test

import (
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/mailroom/services/external/weni/sentenx"
	"github.com/stretchr/testify/assert"
)

const (
	baseURL = "https://sentenx.weni.ai"
)

func TestRequest(t *testing.T) {
	client := sentenx.NewClient(http.DefaultClient, nil, baseURL)

	_, err := client.Request("POST", "", func() {}, nil)
	assert.Error(t, err)

	_, err = client.Request("{[:INVALID:]}", "", nil, nil)
	assert.Error(t, err)

	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		baseURL: {
			httpx.NewMockResponse(400, nil, `[]`),
			httpx.NewMockResponse(200, nil, `{}`),
			httpx.NewMockResponse(400, nil, `{
				"detail": [
					{ "msg": "dummy error message"}
				]
			}`),
		},
	}))

	_, err = client.Request("GET", baseURL, nil, nil)
	assert.Error(t, err)

	_, err = client.Request("GET", baseURL, nil, nil)
	assert.Nil(t, err)

	response := new(interface{})
	_, err = client.Request("GET", baseURL, nil, response)
	assert.Error(t, err)
}
