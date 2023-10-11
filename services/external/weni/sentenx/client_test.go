package sentenx_test

import (
	"fmt"
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

func TestSearch(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		fmt.Sprintf("%s/products/search", baseURL): {
			httpx.NewMockResponse(400, nil, `{
				"detail": [{"msg": "dummy error msg"}, {"msg": "dummy error msg 2"}]
			}`),
			httpx.NewMockResponse(200, nil, `{
				"products": [
					{
            "facebook_id": "1234567891",
            "title": "banana prata 1kg",
            "org_id": "1",
            "channel_id": "5",
            "catalog_id": "asdfgh",
            "product_retailer_id": "p1"
					},
					{
            "facebook_id": "1234567892",
            "title": "doce de banana 250g",
            "org_id": "1",
            "channel_id": "5",
            "catalog_id": "asdfgh",
            "product_retailer_id": "p2"
        	}
				]
			}`),
		},
	}))

	client := sentenx.NewClient(http.DefaultClient, nil, baseURL)

	data := sentenx.NewSearchRequest("banana", "asdfgh", 1.6)

	_, _, err := client.SearchProducts(data)
	assert.EqualError(t, err, "dummy error msg. dummy error msg 2")

	sres, _, err := client.SearchProducts(data)
	assert.NoError(t, err)
	assert.Equal(t, "p1", sres.Products[0].ProductRetailerID)
}
