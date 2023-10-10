package handlers_test

import (
	"fmt"
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/actions"
	"github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestMsgCatalogCreated(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	// add a URN for cathy so we can test all urn sends
	testdata.InsertContactURN(db, testdata.Org1, testdata.Cathy, urns.URN("tel:+12065551212"), 10)

	// delete all URNs for bob
	db.MustExec(`DELETE FROM contacts_contacturn WHERE contact_id = $1`, testdata.Bob.ID)

	msg1 := testdata.InsertIncomingMsg(db, testdata.Org1, testdata.TwilioChannel, testdata.Cathy, "start", models.MsgStatusHandled)

	tcs := []handlers.TestCase{
		{
			Actions: handlers.ContactActionMap{
				testdata.Cathy: []flows.Action{
					actions.NewSendMsgCatalog(
						handlers.NewActionUUID(),
						"", "Some products", "", "View Products", "",
						[]map[string]string{
							{"product_retailer_id": "9f526c6f-b2cb-4457-8048-a7f1dc101e50"},
							{"product_retailer_id": "eb2305cc-bf39-43ad-a069-bbbfb6401acc"},
						},
						false,
						true,
					),
				},
				testdata.George: []flows.Action{
					actions.NewSendMsgCatalog(
						handlers.NewActionUUID(),
						"Select The Service", "", "", "View Products", "",
						[]map[string]string{
							{"product_retailer_id": "cbd9ba07-7156-406e-8006-5b697d18d091"},
							{"product_retailer_id": "63157bd2-6f94-4dbb-b394-ea4eb07ce156"},
						},
						false,
						true,
					),
				},
				testdata.Bob: []flows.Action{
					actions.NewSendMsgCatalog(handlers.NewActionUUID(), "No URNs", "", "", "View Products", "i want a water bottle", nil, false, false),
				},
			},
			Msgs: handlers.ContactMsgMap{
				testdata.Cathy: msg1,
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE contact_id = $1 AND metadata = $2 AND high_priority = TRUE",
					Args:  []interface{}{testdata.Cathy.ID, `{"action":"View Products","body":"Some products","products":["9f526c6f-b2cb-4457-8048-a7f1dc101e50","eb2305cc-bf39-43ad-a069-bbbfb6401acc"]}`},
					Count: 2,
				},
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE contact_id = $1 AND status = 'Q' AND high_priority = FALSE",
					Args:  []interface{}{testdata.George.ID},
					Count: 1,
				},
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE contact_id=$1 AND STATUS = 'F' AND failed_reason = 'D';",
					Args:  []interface{}{testdata.Bob.ID},
					Count: 1,
				},
			},
		},
	}

	handlers.RunTestCases(t, ctx, rt, tcs)

	rc := rp.Get()
	defer rc.Close()

	// Cathy should have 1 batch of queued messages at high priority
	count, err := redis.Int(rc.Do("zcard", fmt.Sprintf("msgs:%s|10/1", testdata.TwilioChannel.UUID)))
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// One bulk for George
	count, err = redis.Int(rc.Do("zcard", fmt.Sprintf("msgs:%s|10/0", testdata.TwilioChannel.UUID)))
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestMsgCatalogSmartCreated(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`
UPDATE public.channels_channel
	SET config='{"threshold":"1.6"}'
WHERE id=$1;`,
		testdata.TwilioChannel.ID)

	tcs := []handlers.TestCase{
		{
			Actions: handlers.ContactActionMap{
				testdata.Cathy: []flows.Action{
					actions.NewSendMsgCatalog(
						handlers.NewActionUUID(),
						"", "Some products", "", "View Products", "Banana",
						nil,
						true,
						true,
					),
				},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE contact_id = $1 AND metadata = $2 AND high_priority = False",
					Args:  []interface{}{testdata.Cathy.ID, `{"action":"View Products","body":"Some products","products":["p1","p2"]}`},
					Count: 1,
				},
			},
		},
	}

	_, err := db.Exec(catalogProductDDL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO public.wpp_products_catalog
	(uuid, facebook_catalog_id, "name", created_on, modified_on, is_active, channel_id, org_id)
	VALUES('2be9092a-1c97-4b24-906f-f0fbe3e1e93e', '123456789', 'Catalog Dummy', now(), now(), true, $1, $2);
	`, testdata.TwilioChannel.ID, testdata.Org1.ID)
	assert.NoError(t, err)

	rt.Config.WeniGPTBaseURL = "https://wenigpt.weni.ai"
	rt.Config.SentenXBaseURL = "https://sentenx.weni.ai"

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		rt.Config.WeniGPTBaseURL: {
			httpx.NewMockResponse(200, nil, `{
				"delayTime": 2,
				"executionTime": 2,
				"id": "66b6a02c-b6e5-4e94-be8b-c631875b24d1",
				"status": "COMPLETED",
				"output": {
					"text": "{\"products\": [\"banana\"]}"
				}
			}`),
		},
		rt.Config.SentenXBaseURL + "/search": {
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

	handlers.RunTestCases(t, ctx, rt, tcs)

	rc := rp.Get()
	defer rc.Close()

	count, err := redis.Int(rc.Do("zcard", fmt.Sprintf("msgs:%s|10/0", testdata.TwilioChannel.UUID)))
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

const (
	catalogProductDDL = `
	CREATE TABLE public.wpp_products_catalog (
		id serial4 NOT NULL,
		uuid uuid NOT NULL,
		facebook_catalog_id varchar(30) NOT NULL,
		"name" varchar(100) NOT NULL,
		created_on timestamptz NOT NULL,
		modified_on timestamptz NOT NULL,
		is_active bool NOT NULL,
		channel_id int4 NOT NULL,
		org_id int4 NOT NULL
	);
`
)
