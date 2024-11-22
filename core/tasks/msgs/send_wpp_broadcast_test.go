package msgs_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/utils"
	_ "github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/msgs"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
)

func TestWppBroadcastTask(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()
	rc := rp.Get()
	defer rc.Close()

	defer testsuite.Reset(testsuite.ResetAll)

	oa, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	assert.NoError(t, err)

	templates, err := oa.Templates()
	assert.NoError(t, err)

	// insert a broadcast so we can check it is being set to sent
	existingID := testdata.InsertBroadcast(db, testdata.Org1, "base", nil, models.NilScheduleID, nil, nil, events.BroadcastTypeDefault)

	doctorsOnly := []models.GroupID{testdata.DoctorsGroup.ID}
	cathyOnly := []models.ContactID{testdata.Cathy.ID}

	// add an extra URN for cathy
	testdata.InsertContactURN(db, testdata.Org1, testdata.Cathy, urns.URN("tel:+12065551212"), 1001)

	// change alexandrias URN to a twitter URN and set her language to eng so that a template gets used for her
	db.MustExec(`UPDATE contacts_contacturn SET identity = 'whatsapp:559899999999', path='559899999999', scheme='whatsapp' WHERE contact_id = $1`, testdata.Alexandria.ID)
	db.MustExec(`UPDATE contacts_contact SET language='eng' WHERE id = $1`, testdata.Alexandria.ID)

	baseMsg := models.WppBroadcastMessage{
		Text: "hello world",
	}

	evaluationMsg := models.WppBroadcastMessage{
		Text: "hello @contact.name",
		Attachments: []utils.Attachment{
			"image/png:http://example.com/image.png",
		},
	}

	replyMsg := models.WppBroadcastMessage{
		Text:            "hello @contact.name, how are you doing today?",
		InteractionType: "reply",
		Header: models.WppBroadcastMessageHeader{
			Type: "text",
			Text: "header for @contact.name",
		},
		Footer: "footer for @contact.name",
		QuickReplies: []string{
			"quick reply 1",
			"quick reply 2",
			"quick reply 3 for @contact.name",
		},
	}

	listMsg := models.WppBroadcastMessage{
		Text:            "hello @contact.name",
		InteractionType: "list",
		ListMessage: flows.ListMessage{
			ButtonText: "button text",
			ListItems: []flows.ListItems{
				{
					Title:       "title 1",
					Description: "description 1",
					UUID:        "123",
				},
			},
		},
	}

	ctaMsg := models.WppBroadcastMessage{
		Text:            "hello @contact.name",
		InteractionType: "cta_url",
		CTAMessage: flows.CTAMessage{
			DisplayText_: "click here",
			URL_:         "http://example.com",
		},
	}

	flowMsg := models.WppBroadcastMessage{
		Text:            "hello @contact.name",
		InteractionType: "flow_msg",
		FlowMessage: flows.FlowMessage{
			FlowID:     "123",
			FlowData:   flows.FlowData{"key": "value"},
			FlowScreen: "screen",
			FlowCTA:    "cta",
			FlowMode:   "draft",
		},
	}

	orderDetailsMsg := models.WppBroadcastMessage{
		Text:            "hello @contact.name",
		InteractionType: "order_details",
		OrderDetails: flows.OrderDetailsMessage{
			ReferenceID: "123",
			PaymentSettings: &flows.OrderPaymentSettings{
				Type:        "digital-goods",
				PaymentLink: "http://example.com",
				PixConfig:   nil,
			},
			TotalAmount: 1000,
			Order: &flows.MessageOrder{
				Items:    nil,
				Subtotal: 0,
				Tax:      nil,
				Shipping: nil,
				Discount: nil,
			},
		},
	}

	templateMsg := models.WppBroadcastMessage{
		Text: "hello @contact.name",
		Template: models.WppBroadcastTemplate{
			UUID: templates[2].UUID(),
			Name: templates[2].Name(),
			Variables: []string{
				"@contact.name",
			},
		},
	}

	tcs := []struct {
		BroadcastID models.BroadcastID
		URNs        []urns.URN
		ContactIDs  []models.ContactID
		GroupIDs    []models.GroupID
		Queue       string
		BatchCount  int
		MsgCount    int
		Msg         models.WppBroadcastMessage
		MsgText     string
	}{
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			doctorsOnly,
			queue.BatchQueue,
			2,
			121,
			baseMsg,
			"hello world",
		},
		{
			existingID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			evaluationMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			evaluationMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			replyMsg,
			"hello Cathy, how are you doing today?",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			listMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			ctaMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			flowMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			nil,
			cathyOnly,
			nil,
			queue.HandlerQueue,
			1,
			1,
			orderDetailsMsg,
			"hello Cathy",
		},
		{
			models.NilBroadcastID,
			[]urns.URN{
				urns.URN("whatsapp:559899999999"),
			},
			nil,
			nil,
			queue.HandlerQueue,
			1,
			1,
			templateMsg,
			"Welcome Alexandia!",
		},
	}

	lastNow := time.Now()
	time.Sleep(10 * time.Millisecond)

	for i, tc := range tcs {
		// handle our start task
		bcast := models.NewWppBroadcast(oa.OrgID(), tc.BroadcastID, tc.Msg, tc.URNs, tc.ContactIDs, tc.GroupIDs)
		err = msgs.CreateWppBroadcastBatches(ctx, rt, bcast)
		assert.NoError(t, err)

		// pop all our tasks and execute them
		var task *queue.Task
		count := 0
		for {
			task, err = queue.PopNextTask(rc, tc.Queue)
			assert.NoError(t, err)
			if task == nil {
				break
			}

			count++
			assert.Equal(t, queue.SendWppBroadcastBatch, task.Type)
			batch := &models.WppBroadcastBatch{}
			err = json.Unmarshal(task.Task, batch)
			assert.NoError(t, err)

			err = msgs.SendWppBroadcastBatch(ctx, rt, batch)
			assert.NoError(t, err)
		}

		// assert our count of batches
		assert.Equal(t, tc.BatchCount, count, "%d: unexpected batch count", i)

		// assert our count of total msgs created
		testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2`, lastNow, tc.MsgText).
			Returns(tc.MsgCount, "%d: unexpected msg count", i)

		// assert our attachments are being sent
		if len(tc.Msg.Attachments) > 0 {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND attachments[1] = $3`, lastNow, tc.MsgText, tc.Msg.Attachments[0]).
				Returns(1, "%d: unexpected attachment count", i)
		}

		// assert our quick replies are being sent
		if len(tc.Msg.QuickReplies) > 0 {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'quick_replies' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected quick reply count", i)
		}

		// assert our header is being sent
		if tc.Msg.Header.Text != "" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'header_text' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected header count", i)
		}

		// assert our footer is being sent
		if tc.Msg.Footer != "" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'footer' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected footer count", i)
		}

		// assert our list items are being sent
		if len(tc.Msg.ListMessage.ListItems) > 0 {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'list_message' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected list message count", i)
		}

		// assert our cta is being sent
		if tc.Msg.InteractionType == "cta_url" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'cta_message' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected cta count", i)
		}

		// assert our flow message is being sent
		if tc.Msg.InteractionType == "flow_msg" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'flow_message' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected flow message count", i)
		}

		// assert our order details message is being sent
		if tc.Msg.InteractionType == "order_details" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata LIKE '%' || 'order_details' || '%'`, lastNow, tc.MsgText).
				Returns(1, "%d: unexpected order details message count", i)
		}

		// assert our template is being sent
		if tc.Msg.Template.UUID != "" {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_msg WHERE org_id = 1 AND created_on > $1 AND text = $2 AND metadata = $3`,
				lastNow,
				tc.MsgText,
				`{"templating":{"template":{"uuid":"17f52732-9655-4230-8225-6bd0800351e1","name":"welcome"},"language":"eng","country":"US","variables":["Alexandia"],"namespace":"7300ee93-b610-4ea5-98f0-f49d66dba123"},"text":"Welcome Alexandia!"}`,
			).Returns(1, "%d: unexpected template count", i)
		}

		// make sure our broadcast is marked as sent
		if tc.BroadcastID != models.NilBroadcastID {
			testsuite.AssertQuery(t, db, `SELECT count(*) FROM msgs_broadcast WHERE id = $1 AND status = 'S'`, tc.BroadcastID).
				Returns(1, "%d: broadcast not marked as sent", i)
		}

		lastNow = time.Now()
		time.Sleep(10 * time.Millisecond)
	}
}
