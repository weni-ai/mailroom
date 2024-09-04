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

	// change alexandrias URN to a twitter URN and set her language to eng so that a template gets used for her
	db.MustExec(`UPDATE contacts_contacturn SET identity = 'twitter:12345', path='12345', scheme='twitter' WHERE contact_id = $1`, testdata.Alexandria.ID)
	db.MustExec(`UPDATE contacts_contact SET language='eng' WHERE id = $1`, testdata.Alexandria.ID)

	msg1 := testdata.InsertIncomingMsg(db, testdata.Org1, testdata.TwilioChannel, testdata.Cathy, "start", models.MsgStatusHandled)

	tcs := []handlers.TestCase{
		{
			Actions: handlers.ContactActionMap{
				testdata.Cathy: []flows.Action{
					actions.NewSendWppMsg(
						handlers.NewActionUUID(),
						"text", "Hi", "", "Hi there.", "footer",
						[]flows.ListItems{{Title: "title", UUID: "62276b09-b712-478c-a658-fcf1c187f4cf", Description: "title description"}},
						"Menu",
						nil,
						"list",
						"",
						"",
						nil,
						"nil",
						true,
					),
				},
				testdata.George: []flows.Action{
					actions.NewSendWppMsg(
						handlers.NewActionUUID(),
						"text", "Hi", "image/png:https://foo.bar.com/images/image1.png", "Hi", "footer",
						[]flows.ListItems{{Title: "title", UUID: "62276b09-b712-478c-a658-fcf1c187f4cf", Description: "title description"}},
						"Menu",
						nil,
						"list",
						"",
						"",
						nil,
						"nil",
						true,
					),
				},
				testdata.Cathy: []flows.Action{
					actions.NewSendWppMsg(
						handlers.NewActionUUID(),
						"text", "Hi", "", "Hi there.", "footer",
						[]flows.ListItems{},
						"Link",
						nil,
						"cta_url",
						"http://foo.bar",
						"",
						nil,
						"nil",
						true,
					),
				},
				testdata.Bob: []flows.Action{
					actions.NewSendWppMsg(handlers.NewActionUUID(), "", "", "", "Text", "footer", []flows.ListItems{}, "Menu", nil, "", "", "", nil, "", false),
				},
				testdata.Cathy: []flows.Action{
					actions.NewSendWppMsg(
						handlers.NewActionUUID(),
						"text", "Hi", "", "Hi there.", "footer",
						[]flows.ListItems{},
						"Start WhatsApp Flow",
						nil,
						"flow_msg",
						"",
						"19472745982745",
						map[string]string{"uuid": string(testdata.Cathy.UUID)},
						"WELCOME_SCREEN",
						true,
					),
				},
			},
			Msgs: handlers.ContactMsgMap{
				testdata.Cathy: msg1,
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE text='Hi there.' AND contact_id = $1 AND high_priority = TRUE",
					Args:  []interface{}{testdata.Cathy.ID},
					Count: 2,
				},
				{
					SQL:   "SELECT COUNT(*) FROM msgs_msg WHERE text='Hi' AND contact_id = $1 AND attachments[1] = $2 AND status = 'Q' AND high_priority = FALSE",
					Args:  []interface{}{testdata.George.ID, "image/png:https://foo.bar.com/images/image1.png"},
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
