package freshchat_test

import (
	"testing"

	_ "github.com/nyaruka/mailroom/services/tickets/freshchat"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"
)

func TestEventCallback(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	testsuite.Reset(testsuite.ResetData | testsuite.ResetStorage)

	defer testsuite.Reset(testsuite.ResetData | testsuite.ResetStorage)

	ticket := testdata.InsertOpenTicket(
		db,
		testdata.Org1,
		testdata.Cathy,
		testdata.Freshchats,
		testdata.DefaultTopic,
		"Have you seen my cookies?",
		"1234567890",
		nil,
	)

	web.RunWebTests(t, ctx, rt, "testdata/event_callback.json", map[string]string{"cathy_ticket_uuid": string(ticket.UUID)})
}
