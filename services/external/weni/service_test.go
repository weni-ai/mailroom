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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService(t *testing.T) {
	_, rt, _, _ := testsuite.Get()

	defer dates.SetNowSource(dates.DefaultNowSource)
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://wenigpt.weni.ai": {
			httpx.NewMockResponse(200, nil, `{
				"delayTime": 2,
				"executionTime": 2,
				"id": "66b6a02c-b6e5-4e94-be8b-c631875b24d1",
				"status": "COMPLETED",
				"output": {
					"text": "weni gpt response output text"
				}
			}`),
		},
		"https://sentenx.weni.ai/products/search": {
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

	catalogService := flows.NewMsgCatalog(static.NewMsgCatalog(assets.MsgCatalogUUID(uuids.New()), "msg_catalog", "msg_catalog", assets.ChannelUUID(uuids.New())))

	svc, err := catalogs.NewService(rt.Config, http.DefaultClient, nil, catalogService, map[string]string{})

	assert.NoError(t, err)

	logger := &flows.HTTPLogger{}

	params := assets.MsgCatalogParam{
		ProductSearch: "",
		ChannelUUID:   uuids.New(),
	}
	call, err := svc.Call(session, params, logger.Log)
	assert.NoError(t, err)
	assert.NotNil(t, call)

	assert.Equal(t, "p1", call.ProductRetailerIDS[0])
	assert.NotNil(t, call.TraceWeniGPT)
	assert.NotNil(t, call.TraceSentenx)

}
