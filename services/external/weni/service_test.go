package catalogs_test

import (
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

func setupMocks() {
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.openai.com/v1/chat/completions": createRepeatedMocks(6, `{
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
		"https://sentenx.weni.ai/products/search": {
			createMockResponse(`{
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
		"https://vtex.com.br/legacy/search/banana": {
			createMockResponse(`[{
				"items": [
					{
						"itemId": "1236"
					}
				]
			}]`),
		},
		"https://vtex.com.br/intelligent/search?hideUnavailableItems=false&locale=pt-BR&query=banana": {
			createMockResponse(`{
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
			createMockResponse(`{
				"products": [
					{
						"items": [
							{
								"itemId": "1234",
								"sellers": [
									{
										"sellerId": "2",
										"sellerDefault": true,
										"commertialOffer": {
											"AvailableQuantity": 10
										}
									}
								]
							}
						]
					}
				]
			}`),
			createMockResponse(`{
				"products": [
					{
						"items": [
							{
								"itemId": "1234",
								"sellers": [
									{
										"sellerId": "2",
										"sellerDefault": true,
										"commertialOffer": {
											"AvailableQuantity": 10
										}
									}
								]
							}
						]
					}
				]
			}`),
		},
		"https://api.linximpulse.com/engage/search/v3/search?apiKey=1234567890&secretKey=1234567890&terms=banana": {
			createMockResponse(`{
				"products": [
					{
						"id": "12346",
						"skus": [
							{
								"sku": "1234",
								"properties": {
									"status": "available",
									"stock": 10
								}
							}
						]
					}
				]
			}`),
		},
		"https://vtex.com.br/intelligent/search?hideUnavailableItems=true&locale=pt-BR&query=banana": {
			createMockResponse(`{
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
		"https://graph.facebook.com/v14.0/123456789/products?access_token=&fields=%5B%22category%22%2C%22name%22%2C%22retailer_id%22%2C%22availability%22%5D&filter=%7B%22or%22%3A%5B%7B%22and%22%3A%5B%7B%22retailer_id%22%3A%7B%22i_contains%22%3A%22p1%22%7D%7D%2C%7B%22availability%22%3A%7B%22i_contains%22%3A%22in+stock%22%7D%7D%2C%7B%22visibility%22%3A%7B%22i_contains%22%3A%22published%22%7D%7D%5D%7D%5D%7D&summary=true": {
			createMockResponse(`{
				"data": [
					{
						"name": "banana prata (Kg)",
						"retailer_id": "p1",
						"availability": "in stock",
						"visibility": "published",
						"id": "111111222233333"
					}
				]
			}`),
		},
		"https://graph.facebook.com/v14.0/123456789/products?access_token=&fields=%5B%22category%22%2C%22name%22%2C%22retailer_id%22%2C%22availability%22%5D&filter=%7B%22or%22%3A%5B%7B%22and%22%3A%5B%7B%22retailer_id%22%3A%7B%22i_contains%22%3A%221236%22%7D%7D%2C%7B%22availability%22%3A%7B%22i_contains%22%3A%22in+stock%22%7D%7D%2C%7B%22visibility%22%3A%7B%22i_contains%22%3A%22published%22%7D%7D%5D%7D%5D%7D&summary=true": {
			createMockResponse(`{
				"data": [
					{
						"name": "banana prata (Kg)",
						"retailer_id": "1236",
						"availability": "in stock",
						"visibility": "published",
						"id": "111111222233333"
					}
				]
			}`),
		},
		"https://graph.facebook.com/v14.0/123456789/products?access_token=&fields=%5B%22category%22%2C%22name%22%2C%22retailer_id%22%2C%22availability%22%5D&filter=%7B%22or%22%3A%5B%7B%22and%22%3A%5B%7B%22retailer_id%22%3A%7B%22i_contains%22%3A%221234%2310%22%7D%7D%2C%7B%22availability%22%3A%7B%22i_contains%22%3A%22in+stock%22%7D%7D%2C%7B%22visibility%22%3A%7B%22i_contains%22%3A%22published%22%7D%7D%5D%7D%5D%7D&summary=true": createRepeatedMocks(3, `{
				"data": [
					{
						"name": "banana prata (Kg)",
						"retailer_id": "1234#10",
						"availability": "in stock",
						"visibility": "published",
						"id": "111111222233333"
					}
				]
			}`),
		"https://graph.facebook.com/v14.0/123456789/products?access_token=&fields=%5B%22category%22%2C%22name%22%2C%22retailer_id%22%2C%22availability%22%5D&filter=%7B%22or%22%3A%5B%7B%22and%22%3A%5B%7B%22retailer_id%22%3A%7B%22i_contains%22%3A%221234%232%22%7D%7D%2C%7B%22availability%22%3A%7B%22i_contains%22%3A%22in+stock%22%7D%7D%2C%7B%22visibility%22%3A%7B%22i_contains%22%3A%22published%22%7D%7D%5D%7D%5D%7D&summary=true": {
			createMockResponse(`{
								"data": [
									{
										"name": "banana prata (Kg)",
										"retailer_id": "1234#2",
										"availability": "in stock",
										"visibility": "published",
										"id": "111111222233333"
									}
								]
							}`),
		},
		"https://graph.facebook.com/v14.0/123456789/products?access_token=&fields=%5B%22category%22%2C%22name%22%2C%22retailer_id%22%2C%22availability%22%5D&filter=%7B%22or%22%3A%5B%7B%22and%22%3A%5B%7B%22retailer_id%22%3A%7B%22i_contains%22%3A%221234%22%7D%7D%2C%7B%22availability%22%3A%7B%22i_contains%22%3A%22in+stock%22%7D%7D%2C%7B%22visibility%22%3A%7B%22i_contains%22%3A%22published%22%7D%7D%5D%7D%5D%7D&summary=true": {
			createMockResponse(`{
				"data": [
					{
						"name": "banana prata (Kg)",
						"retailer_id": "1234",
						"availability": "in stock",
						"visibility": "published",
						"id": "111111222233333"
					}
				]
			}`),
		},
		"https://vtex.com.br/intelligent/searchapi/checkout/pub/orderForms/simulation?test=test&deliveryChannel=delivery": {
			createMockResponse(`{
				"items": [
					{
						"id": "1234",
						"availability": "available"
					}
				],
				"logisticsInfo": [
					{
						"itemIndex": 0,
						"deliveryChannels": [
							{
								"id": "delivery"
							}
						]
					}
				]
			}`),
		},
		"http://vtex.com.br/api/io/_v/api/intelligent-search/sponsored_products?hideUnavailableItems=true&locale=pt-BR&query=banana": {
			createMockResponse(`[{
				"items": [
					{
						"itemId": "1234"
					}
				]
			}]`),
		},
	}))
}

func createMockResponse(body string) httpx.MockResponse {
	return httpx.NewMockResponse(200, nil, body)
}

func createRepeatedMocks(count int, body string) []httpx.MockResponse {
	mocks := make([]httpx.MockResponse, count)
	for i := 0; i < count; i++ {
		mocks[i] = createMockResponse(body)
	}
	return mocks
}

func TestService(t *testing.T) {
	defer dates.SetNowSource(dates.DefaultNowSource)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	_, rt, db, _ := testsuite.Get()
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	setupMocks()

	catalogs.SetDB(db)
	catalogService := flows.NewMsgCatalog(static.NewMsgCatalog(assets.MsgCatalogUUID(testdata.Org1.UUID), "msg_catalog", "msg_catalog", assets.ChannelUUID(uuids.New())))

	svc, err := catalogs.NewService(rt.Config, http.DefaultClient, nil, catalogService, map[string]string{})
	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	params := assets.MsgCatalogParam{
		ProductSearch: "banana",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "default",
		SearchUrl:     "",
		ApiType:       "",
		PostalCode:    "",
	}
	call, err := svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "p1", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)

	params = assets.MsgCatalogParam{
		ProductSearch: "banana",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "vtex",
		SearchUrl:     "https://vtex.com.br/legacy/search",
		ApiType:       "legacy",
		PostalCode:    "",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1236", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)
	params = assets.MsgCatalogParam{
		ProductSearch:        "banana",
		ChannelUUID:          uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:           "vtex",
		SearchUrl:            "https://vtex.com.br/intelligent/search",
		ApiType:              "intelligent",
		PostalCode:           "000000-000",
		SellerId:             "10",
		CartSimulationParams: "test=test&deliveryChannel=delivery",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1234#10", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)

	params = assets.MsgCatalogParam{
		ProductSearch: "",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "vtex",
		SearchUrl:     "https://vtex.com.br/intelligent/search",
		ApiType:       "intelligent",
		PostalCode:    "",
		SellerId:      "",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1234#2", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)

	params = assets.MsgCatalogParam{
		ProductSearch:   "banana",
		ChannelUUID:     uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:      "vtex",
		SearchUrl:       "https://vtex.com.br/intelligent/search",
		ApiType:         "intelligent",
		PostalCode:      "",
		SellerId:        "10",
		HasVtexAds:      true,
		ExtraPrompt:     "Test Prompt",
		HideUnavailable: true,
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1234#10", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)

	params = assets.MsgCatalogParam{
		ProductSearch: "banana",
		ChannelUUID:   uuids.UUID(testdata.TwilioChannel.UUID),
		SearchType:    "vtex",
		SearchUrl:     "https://api.linximpulse.com/engage/search/v3/search?apiKey=1234567890&secretKey=1234567890",
		ApiType:       "linx",
		PostalCode:    "",
		SellerId:      "",
		HasVtexAds:    false,
		ExtraPrompt:   "",
	}
	call, err = svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)
	assert.Equal(t, "1234", call.ProductRetailerIDS[0].ProductRetailerIDs[0])
	assert.NotNil(t, call.Traces)
	assert.Equal(t, []string{"banana"}, call.SearchKeywords)
}
