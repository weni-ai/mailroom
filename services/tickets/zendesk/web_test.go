package zendesk

import (
	"testing"

	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"
)

func TestChannelback(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	// create a zendesk ticket for Cathy
	ticket := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "1234", nil)

	web.RunWebTests(t, ctx, rt, "testdata/channelback.json", map[string]string{"cathy_ticket_uuid": string(ticket.UUID)})
}

func TestEventCallback(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll) // tests include destroying ticketer

	// create a zendesk ticket for Cathy
	ticket := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "1234", nil)

	web.RunWebTests(t, ctx, rt, "testdata/event_callback.json", map[string]string{"cathy_ticket_uuid": string(ticket.UUID)})
}

func TestWebhook(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	// create a zendesk ticket for Cathy
	ticket := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "1234", nil)

	web.RunWebTests(t, ctx, rt, "testdata/webhook.json", map[string]string{"cathy_ticket_uuid": string(ticket.UUID)})
}

func TestCSAT(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	// create a zendesk ticket for Cathy
	ticket := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "1234", nil)

	web.RunWebTests(t, ctx, rt, "testdata/csat.json", map[string]string{"cathy_ticket_uuid": string(ticket.UUID)})

}
