package msg_test

import (
	"fmt"
	"testing"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"
)

func TestServer(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)

	cathyIn := testdata.InsertIncomingMsg(db, testdata.Org1, testdata.TwilioChannel, testdata.Cathy, "hello", models.MsgStatusHandled)
	cathyOut := testdata.InsertOutgoingMsg(db, testdata.Org1, testdata.TwilioChannel, testdata.Cathy, "how can we help", nil, models.MsgStatusSent, false)
	bobOut := testdata.InsertOutgoingMsg(db, testdata.Org1, testdata.VonageChannel, testdata.Bob, "this failed", nil, models.MsgStatusFailed, false)

	web.RunWebTests(t, ctx, rt, "testdata/resend.json", map[string]string{
		"cathy_msgin_id":  fmt.Sprintf("%d", cathyIn.ID()),
		"cathy_msgout_id": fmt.Sprintf("%d", cathyOut.ID()),
		"bob_msgout_id":   fmt.Sprintf("%d", bobOut.ID()),
	})
}

func TestSend(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)
	testsuite.Reset(testsuite.ResetData)

	projectUUID := "11111111-1111-4111-8111-111111111111"
	db.MustExec(`CREATE TABLE IF NOT EXISTS internal_project (
		id SERIAL PRIMARY KEY,
		project_uuid UUID NOT NULL,
		org_ptr_id INTEGER NOT NULL
	)`)
	db.MustExec(`DELETE FROM internal_project WHERE project_uuid = $1`, projectUUID)
	db.MustExec(`INSERT INTO internal_project (project_uuid, org_ptr_id) VALUES ($1, $2)`, projectUUID, testdata.Org1.ID)

	web.RunWebTests(t, ctx, rt, "testdata/send.json", map[string]string{
		"project_uuid": projectUUID,
		"cathy_urn":    string(testdata.Cathy.URN),
	})
}
