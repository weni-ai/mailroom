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

func TestSync(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetData)

	prevToken := rt.Config.AuthToken
	rt.Config.AuthToken = "sesame"
	defer func() { rt.Config.AuthToken = prevToken }()

	openZD := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "open in both", "101", nil)
	solvedZD := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Bob, testdata.Zendesk, testdata.DefaultTopic, "closed in zd", "102", nil)
	missingZD := testdata.InsertOpenTicket(db, testdata.Org1, testdata.George, testdata.Zendesk, testdata.DefaultTopic, "no external id", "", nil)
	goneZD := testdata.InsertOpenTicket(db, testdata.Org1, testdata.Alexandria, testdata.Zendesk, testdata.DefaultTopic, "missing in zd", "103", nil)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "already closed", "199", nil)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Mailgun, testdata.DefaultTopic, "other ticketer", "999", nil)

	web.RunWebTests(t, ctx, rt, "testdata/sync.json", map[string]string{
		"open_uuid":    string(openZD.UUID),
		"solved_uuid":  string(solvedZD.UUID),
		"missing_uuid": string(missingZD.UUID),
		"gone_uuid":    string(goneZD.UUID),
	})
}
