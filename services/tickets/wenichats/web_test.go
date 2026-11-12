package wenichats_test

import (
	"testing"

	"github.com/nyaruka/mailroom/core/presign"
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
		testdata.Wenichats,
		testdata.DefaultTopic,
		"Have you seen my cookies?",
		"e0fa6b4b-92c2-4906-98dc-e1a9f6b141d2",
		nil,
	)

	testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")
	photo := presign.DurableURL("https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg")

	web.RunWebTests(t, ctx, rt, "testdata/event_callback.json", map[string]string{
		"cathy_ticket_uuid": string(ticket.UUID),
		"durable_photo":     photo,
	})
}
