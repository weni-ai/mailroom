package ticket

import (
	"testing"

	"github.com/nyaruka/mailroom/core/models"
	_ "github.com/nyaruka/mailroom/services/tickets/mailgun"
	"github.com/nyaruka/mailroom/services/tickets/wenichats"
	_ "github.com/nyaruka/mailroom/services/tickets/zendesk"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"
	"github.com/stretchr/testify/require"
)

func TestTicketAssign(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "17", testdata.Admin)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "21", testdata.Agent)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "34", nil)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Bob, testdata.Internal, testdata.DefaultTopic, "", "", nil)

	web.RunWebTests(t, ctx, rt, "testdata/assign.json", nil)
}

func TestTicketAddNote(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "17", testdata.Admin)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "21", testdata.Agent)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "34", nil)

	web.RunWebTests(t, ctx, rt, "testdata/add_note.json", nil)
}

func TestTicketChangeTopic(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.DefaultTopic, "Have you seen my cookies?", "17", testdata.Admin)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.SupportTopic, "Have you seen my cookies?", "21", testdata.Agent)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Internal, testdata.SalesTopic, "Have you seen my cookies?", "34", nil)

	web.RunWebTests(t, ctx, rt, "testdata/change_topic.json", nil)
}

func TestTicketClose(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	// create 2 open tickets and 1 closed one for Cathy across two different ticketers
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Mailgun, testdata.DefaultTopic, "Have you seen my cookies?", "17", testdata.Admin)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "21", nil)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "34", testdata.Editor)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "21", nil)

	web.RunWebTests(t, ctx, rt, "testdata/close.json", nil)
}

func TestTicketReopen(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	// create 2 closed tickets and 1 open one for Cathy
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Mailgun, testdata.DefaultTopic, "Have you seen my cookies?", "17", testdata.Admin)
	testdata.InsertClosedTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "21", nil)
	testdata.InsertOpenTicket(db, testdata.Org1, testdata.Cathy, testdata.Zendesk, testdata.DefaultTopic, "Have you seen my cookies?", "34", testdata.Editor)

	web.RunWebTests(t, ctx, rt, "testdata/reopen.json", nil)
}

func TestOpenTicket(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	db.MustExec(`UPDATE orgs_org SET config = '{"is_multi_agents": true}' WHERE id = $1`, testdata.Org1.ID)
	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)
	require.True(t, oa.Org().ConfigBoolValue("is_multi_agents", false))

	wenichats.SetDB(rt.DB)
	web.RunWebTests(t, ctx, rt, "testdata/open.json", nil)
}
