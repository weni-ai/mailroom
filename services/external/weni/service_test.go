package catalogs_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	catalogs "github.com/nyaruka/mailroom/services/external/weni"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	_, rt, db, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.openai.com/v1/chat/completions": {
			httpx.NewMockResponse(200, nil, `{
				"id": "chatcmpl-7IfBIQsTVKbwOiHPgcrpthaCn7K1t",
				"object": "chat.completion",
				"created":1684682560,
				"model":"gpt-3.5-turbo-0301",
				"usage":{
					"prompt_tokens":26,
					"completion_tokens":8,
					"total_tokens":34
				},
				"choices":[
					{
						"message":{
							"role":"assistant",
							"content":"{\"products\": [\"banana\"]}"
						},
						"finish_reason":"stop",
						"index":0
					}
				]
			}`),
		},
		"https://sentenx.weni.ai/products/search": {
			httpx.NewMockResponse(200, nil, `{
				"products": [
					{
            "facebook_id": "1234567891",
            "title": "banana prata 1kg",
            "org_id": "1",
            "channel_id": "10000",
            "catalog_id": "123456789",
            "product_retailer_id": "p1"
					}
				]
			}`),
		},
		"https://vtex.com.br/legacy/search": {
			httpx.NewMockResponse(200, nil, `{
				[
					{
						"items": [
							{
								"itemId": "1236"
							}
						]
					}
				]
			}`),
		},
		"https://vtex.com.br/intelligent/search": {
			httpx.NewMockResponse(200, nil, `{
				"products": [
					{
						"items": [
							{
								"itemId": "1234"
							}
						]
					}
					
				]
			}`),
		},
	}))
	catalogs.SetDB(db)
	catalogService := flows.NewMsgCatalog(static.NewMsgCatalog(assets.MsgCatalogUUID(testdata.Org1.UUID), "msg_catalog", "msg_catalog", assets.ChannelUUID(uuids.New())))

	svc, err := catalogs.NewService(rt.Config, http.DefaultClient, nil, catalogService, map[string]string{})

	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	params := assets.MsgCatalogParam{
		ProductSearch: "",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "default",
		SearchUrl:     "",
		ApiType:       "",
		SellerId:      "",
	}
	call, err := svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	fmt.Println(call.ProductRetailerIDS)
	assert.Equal(t, "p1", call.ProductRetailerIDS["banana"])
	assert.NotNil(t, call.Traces)

	params = assets.MsgCatalogParam{
		ProductSearch: "",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "vtex",
		SearchUrl:     "https://vtex.com.br/legacy/search",
		ApiType:       "legacy",
		SellerId:      "",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1236", call.ProductRetailerIDS["banana"])
	assert.NotNil(t, call.Traces)

	params = assets.MsgCatalogParam{
		ProductSearch: "",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "vtex",
		SearchUrl:     "https://vtex.com.br/intelligent/search",
		ApiType:       "intelligent",
		SellerId:      "",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1234", call.ProductRetailerIDS["banana"])
	assert.NotNil(t, call.Traces)
}
