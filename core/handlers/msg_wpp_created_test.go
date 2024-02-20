package handlers_test

import (
	"fmt"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/actions"
	"github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
)

func TestMsgWppCreated(t *testing.T) {
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
					actions.NewSendWppMsg(
						handlers.NewActionUUID(),
						"text", "Hi", "", "Hi there.", "Footer",
						[]flows.ListItems{},
						"",
						"",
						nil,
						"",
						true,
					),
				},
				testdata.Bob: []flows.Action{
					actions.NewSendWppMsg(handlers.NewActionUUID(), "", "", "", "Text", "footer", []flows.ListItems{}, "", "", nil, "", false),
				},
			},
			Msgs: handlers.ContactMsgMap{
				testdata.Cathy: msg1,
			},
			SQLAssertions: []handlers.SQLAssertion{
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
