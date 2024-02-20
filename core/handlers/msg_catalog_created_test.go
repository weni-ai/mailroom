package handlers_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
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

	db.MustExec(`INSERT INTO msgs_msg(uuid, org_id, channel_id, contact_id, contact_urn_id, text, direction, status, created_on, visibility, msg_count, error_count, next_attempt) 
	VALUES($1,   $2,     $3,         $4,         $5,             $6,   $7,        $8,     $9,         'V',        1,         0,           NOW())`,
		uuids.New(), testdata.Org1.ID, testdata.TwilioChannel.ID, testdata.George.ID, testdata.George.URNID, "Hi", models.DirectionOut, models.MsgStatusQueued, time.Now())

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
						"",
						"",
						"",
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
						"",
						"",
						"",
						true,
					),
				},
				testdata.Bob: []flows.Action{
					actions.NewSendMsgCatalog(handlers.NewActionUUID(), "No URNs", "", "", "View Products", "i want a water bottle", nil, false, "", "", "", false),
				},
			},
			Msgs: handlers.ContactMsgMap{
				testdata.Cathy: msg1,
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE contact_id = $1 AND status = 'Q' AND high_priority = FALSE",
					Args:  []interface{}{testdata.George.ID},
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
