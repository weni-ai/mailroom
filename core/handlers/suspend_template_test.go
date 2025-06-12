package handlers_test

import (
	"testing"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
)

func TestSuspendTemplate(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`UPDATE orgs_org SET config = '{"suspend_template": true}' WHERE id = $1`, testdata.Org1.ID)

	orgAssets, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	if err != nil {
		t.Fatalf("error getting org assets: %s", err)
	}

	templateUUID := assets.TemplateUUID("17f52732-9655-4230-8225-6bd0800351e1")

	// Create broadcast message with a template
	msg := models.WppBroadcastMessage{
		Text: "Hello",
		Template: models.WppBroadcastTemplate{
			UUID:      templateUUID,
			Name:      "welcome",
			Variables: []string{"First Variable"},
			Locale:    "eng-US",
		},
	}

	// Create a broadcast
	broadcast := models.NewWppBroadcast(
		testdata.Org1.ID,
		models.NilBroadcastID,
		msg,
		[]urns.URN{urns.URN("whatsapp:5511999999999")},
		[]models.ContactID{testdata.George.ID},
		nil,
		testdata.WhatsAppCloudChannel.ID,
		queue.WppBroadcastBatchQueue,
	)

	batch := broadcast.CreateBatch([]models.ContactID{testdata.George.ID})

	msgs, err := models.CreateWppBroadcastMessages(ctx, rt, orgAssets, batch)

	if err != nil {
		t.Errorf("Error creating messages: %s", err)
	}

	if len(msgs) != 1 {
		t.Errorf("Expected 1 message, got %d", len(msgs))
	} else {
		if msgs[0].Status() != models.MsgStatusFailed {
			t.Errorf("Expected message status to be failed, got %s", msgs[0].Status())
		}

		if msgs[0].FailedReason() != "T" {
			t.Errorf("Expected failed reason to be 'T', got %s", msgs[0].FailedReason())
		}
	}
}
