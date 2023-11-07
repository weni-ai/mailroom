package models_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignSchedule(t *testing.T) {
	eastern, _ := time.LoadLocation("US/Eastern")
	nilDate := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)

	tcs := []struct {
		Offset       int
		Unit         models.OffsetUnit
		DeliveryHour int

		Timezone *time.Location
		Now      time.Time
		Start    time.Time

		HasError  bool
		Scheduled time.Time
		Delta     time.Duration
	}{
		// this crosses a DST boundary, so two days is really 49 hours (fall back)
		{2, models.OffsetDay, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 11, 3, 0, 30, 0, 0, eastern),
			false, time.Date(2029, 11, 5, 0, 30, 0, 0, eastern), time.Hour * 49},

		// this also crosses a boundary but in the other direction
		{2, models.OffsetDay, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 3, 10, 2, 30, 0, 0, eastern),
			false, time.Date(2029, 3, 12, 2, 30, 0, 0, eastern), time.Hour * 47},

		// this event is in the past, no schedule
		{2, models.OffsetDay, models.NilDeliveryHour, eastern, time.Date(2018, 10, 31, 0, 0, 0, 0, eastern), time.Date(2018, 10, 15, 0, 0, 0, 0, eastern),
			false, nilDate, 0},

		{2, models.OffsetMinute, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 1, 1, 2, 58, 0, 0, eastern),
			false, time.Date(2029, 1, 1, 3, 0, 0, 0, eastern), time.Minute * 2},

		{2, models.OffsetMinute, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 1, 1, 2, 57, 32, 0, eastern),
			false, time.Date(2029, 1, 1, 3, 0, 0, 0, eastern), time.Minute*2 + time.Second*28},

		{-2, models.OffsetHour, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 1, 2, 1, 58, 0, 0, eastern),
			false, time.Date(2029, 1, 1, 23, 58, 0, 0, eastern), time.Hour * -2},

		{2, models.OffsetWeek, models.NilDeliveryHour, eastern, time.Now(), time.Date(2029, 1, 20, 1, 58, 0, 0, eastern),
			false, time.Date(2029, 2, 3, 1, 58, 0, 0, eastern), time.Hour * 24 * 14},

		{2, models.OffsetWeek, 14, eastern, time.Now(), time.Date(2029, 1, 20, 1, 58, 0, 0, eastern),
			false, time.Date(2029, 2, 3, 14, 0, 0, 0, eastern), time.Hour*24*14 + 13*time.Hour - 58*time.Minute},

		{2, "L", 14, eastern, time.Now(), time.Date(2029, 1, 20, 1, 58, 0, 0, eastern),
			true, nilDate, 0},
	}

	for i, tc := range tcs {
		evtJSON := fmt.Sprintf(`{"offset": %d, "unit": "%s", "delivery_hour": %d}`, tc.Offset, tc.Unit, tc.DeliveryHour)
		evt := &models.CampaignEvent{}
		err := json.Unmarshal([]byte(evtJSON), evt)
		require.NoError(t, err)

		scheduled, err := evt.ScheduleForTime(tc.Timezone, tc.Now, tc.Start)

		if err != nil {
			assert.True(t, tc.HasError, "%d: received unexpected error %s", i, err.Error())
		} else if tc.Scheduled.IsZero() {
			assert.Nil(t, scheduled, "%d: received unexpected value", i)
		} else {
			assert.Equal(t, tc.Scheduled.In(time.UTC), scheduled.In(time.UTC), "%d: mismatch in expected scheduled and actual", i)
			assert.Equal(t, scheduled.Sub(tc.Start), tc.Delta, "%d: mismatch in expected delta", i)
		}
	}
}

func TestAddEventFires(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer db.MustExec(`DELETE FROM campaigns_eventfire`)

	scheduled1 := time.Date(2020, 9, 8, 14, 38, 30, 123456789, time.UTC)

	err := models.AddEventFires(ctx, db, []*models.FireAdd{
		{ContactID: testdata.Cathy.ID, EventID: testdata.RemindersEvent1.ID, Scheduled: scheduled1},
		{ContactID: testdata.Bob.ID, EventID: testdata.RemindersEvent1.ID, Scheduled: scheduled1},
		{ContactID: testdata.Bob.ID, EventID: testdata.RemindersEvent2.ID, Scheduled: scheduled1},
	})
	require.NoError(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire`).Returns(3)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire WHERE contact_id = $1 AND event_id = $2`, testdata.Cathy.ID, testdata.RemindersEvent1.ID).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire WHERE contact_id = $1 AND event_id = $2`, testdata.Bob.ID, testdata.RemindersEvent1.ID).Returns(1)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire WHERE contact_id = $1 AND event_id = $2`, testdata.Bob.ID, testdata.RemindersEvent2.ID).Returns(1)

	db.MustExec(`UPDATE campaigns_eventfire SET fired = NOW() WHERE contact_id = $1`, testdata.Cathy.ID)

	scheduled2 := time.Date(2020, 9, 8, 14, 38, 30, 123456789, time.UTC)

	err = models.AddEventFires(ctx, db, []*models.FireAdd{
		{ContactID: testdata.Cathy.ID, EventID: testdata.RemindersEvent1.ID, Scheduled: scheduled2}, // fine because previous one now has non-null fired
		{ContactID: testdata.Bob.ID, EventID: testdata.RemindersEvent1.ID, Scheduled: scheduled2},   // won't be added due to conflict
	})
	require.NoError(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire`).Returns(4)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire WHERE contact_id = $1 AND event_id = $2`, testdata.Cathy.ID, testdata.RemindersEvent1.ID).Returns(2)
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM campaigns_eventfire WHERE contact_id = $1`, testdata.Bob.ID).Returns(2)
}

func TestDeleteEventContactFiresFromGroups(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetDB)

	// create a test group
	testdata.InsertContactGroup(db, testdata.Org1, "94c816d7-cc87-42db-a577-ce072ceaab81", "GroupTest", "")

	// create a campaign related to the created group
	db.MustExec(`INSERT INTO public.campaigns_campaign
		(id, is_active, created_on, modified_on, uuid, "name", is_archived, created_by_id, group_id, modified_by_id, org_id)
		VALUES(30001, true, '2021-12-20 11:31:42.996', '2021-12-20 11:31:42.996', '72aa12c5-cc11-4bc7-9406-044047845c72', 'Doctor Reminders 2', false, 2, 30000, 2, 1);
	`)
	db.MustExec(`INSERT INTO public.campaigns_campaignevent
	(id, is_active, created_on, modified_on, uuid, event_type, "offset", unit, start_mode, message, delivery_hour, campaign_id, created_by_id, flow_id, modified_by_id, relative_to_id)
	VALUES(30001, true, '2021-12-20 11:31:43.038', '2021-12-20 11:31:43.038', 'aff4b8ac-2534-420f-a353-66a3e74b6e10', 'M', 10, 'M', 'I', '"eng"=>"Hi @contact.name, it is time to consult with your patients.", "fra"=>"Bonjour @contact.name, il est temps de consulter vos patients."', -1, 30001, 2, 10009, 2, 8);
	`)
	// insert a event fire for cathy from  the created event
	db.MustExec(`INSERT INTO public.campaigns_eventfire
	(id, scheduled, fired, fired_result, contact_id, event_id)
	VALUES(1, '2023-11-07 11:03:18.416', NULL, NULL, 10000, 30001);`)

	// previous event fire created must be present here as 1 row returned
	testsuite.AssertQuery(t, db, `SELECT count(*) FROM public.campaigns_eventfire`).Returns(1)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshNone)
	require.NoError(t, err)

	_, cathy := testdata.Cathy.Load(db, oa)

	err = models.DeleteUnfiredContactEventsFromGroups(ctx, db, []models.ContactID{models.ContactID(cathy.ID())}, []models.GroupID{models.GroupID(30000)})
	require.NoError(t, err)

	// after delete the event fire returned rows must be 0
	testsuite.AssertQuery(t, db, `SELECT count(*)	FROM public.campaigns_eventfire;`).Returns(0)
}
