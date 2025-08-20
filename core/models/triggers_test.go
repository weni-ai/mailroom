package models_test

import (
	"testing"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTriggers(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`DELETE FROM triggers_trigger`)
	farmersGroup := testdata.InsertContactGroup(db, testdata.Org1, assets.GroupUUID(uuids.New()), "Farmers", "")

	// create trigger for other org to ensure it isn't loaded
	testdata.InsertCatchallTrigger(db, testdata.Org2, testdata.Org2Favorites, nil, nil)

	tcs := []struct {
		id               models.TriggerID
		type_            models.TriggerType
		flowID           models.FlowID
		keyword          string
		keywordMatchType models.MatchType
		referrerID       string
		includeGroups    []models.GroupID
		excludeGroups    []models.GroupID
		includeContacts  []models.ContactID
		channelID        models.ChannelID
		createdOn        time.Time
	}{
		{
			id:               testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.Favorites, "join", models.MatchFirst, nil, nil),
			type_:            models.KeywordTriggerType,
			flowID:           testdata.Favorites.ID,
			keyword:          "join",
			keywordMatchType: models.MatchFirst,
			createdOn:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id: testdata.InsertKeywordTrigger(
				db, testdata.Org1, testdata.PickANumber, "start", models.MatchOnly,
				[]*testdata.Group{testdata.DoctorsGroup, testdata.TestersGroup},
				[]*testdata.Group{farmersGroup},
			),
			type_:            models.KeywordTriggerType,
			flowID:           testdata.PickANumber.ID,
			keyword:          "start",
			keywordMatchType: models.MatchOnly,
			includeGroups:    []models.GroupID{testdata.DoctorsGroup.ID, testdata.TestersGroup.ID},
			excludeGroups:    []models.GroupID{farmersGroup.ID},
			createdOn:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:            testdata.InsertIncomingCallTrigger(db, testdata.Org1, testdata.Favorites, []*testdata.Group{testdata.DoctorsGroup, testdata.TestersGroup}, []*testdata.Group{farmersGroup}),
			type_:         models.IncomingCallTriggerType,
			flowID:        testdata.Favorites.ID,
			includeGroups: []models.GroupID{testdata.DoctorsGroup.ID, testdata.TestersGroup.ID},
			excludeGroups: []models.GroupID{farmersGroup.ID},
			createdOn:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:        testdata.InsertMissedCallTrigger(db, testdata.Org1, testdata.Favorites),
			type_:     models.MissedCallTriggerType,
			flowID:    testdata.Favorites.ID,
			createdOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:        testdata.InsertNewConversationTrigger(db, testdata.Org1, testdata.Favorites, testdata.TwilioChannel),
			type_:     models.NewConversationTriggerType,
			flowID:    testdata.Favorites.ID,
			channelID: testdata.TwilioChannel.ID,
			createdOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:        testdata.InsertReferralTrigger(db, testdata.Org1, testdata.Favorites, "", nil),
			type_:     models.ReferralTriggerType,
			flowID:    testdata.Favorites.ID,
			createdOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:         testdata.InsertReferralTrigger(db, testdata.Org1, testdata.Favorites, "3256437635", testdata.TwilioChannel),
			type_:      models.ReferralTriggerType,
			flowID:     testdata.Favorites.ID,
			referrerID: "3256437635",
			channelID:  testdata.TwilioChannel.ID,
			createdOn:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			id:        testdata.InsertCatchallTrigger(db, testdata.Org1, testdata.Favorites, nil, nil),
			type_:     models.CatchallTriggerType,
			flowID:    testdata.Favorites.ID,
			createdOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	require.Equal(t, len(tcs), len(oa.Triggers()))

	for i, tc := range tcs {
		actual := oa.Triggers()[i]

		assert.Equal(t, tc.id, actual.ID(), "id mismatch in trigger #%d", i)
		assert.Equal(t, tc.type_, actual.TriggerType(), "type mismatch in trigger #%d", i)
		assert.Equal(t, tc.flowID, actual.FlowID(), "flow id mismatch in trigger #%d", i)
		assert.Equal(t, tc.keyword, actual.Keyword(), "keyword mismatch in trigger #%d", i)
		assert.Equal(t, tc.keywordMatchType, actual.MatchType(), "match type mismatch in trigger #%d", i)
		assert.Equal(t, tc.referrerID, actual.ReferrerID(), "referrer id mismatch in trigger #%d", i)
		assert.ElementsMatch(t, tc.includeGroups, actual.IncludeGroupIDs(), "include groups mismatch in trigger #%d", i)
		assert.ElementsMatch(t, tc.excludeGroups, actual.ExcludeGroupIDs(), "exclude groups mismatch in trigger #%d", i)
		assert.ElementsMatch(t, tc.includeContacts, actual.ContactIDs(), "include contacts mismatch in trigger #%d", i)
		assert.Equal(t, tc.channelID, actual.ChannelID(), "channel id mismatch in trigger #%d", i)
	}
}

func TestFindMatchingMsgTrigger(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`DELETE FROM triggers_trigger`)

	joinID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.Favorites, "join", models.MatchFirst, nil, nil)
	resistID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.SingleMessage, "resist", models.MatchOnly, nil, nil)
	emojiID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.PickANumber, "👍", models.MatchFirst, nil, nil)
	doctorsID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.SingleMessage, "resist", models.MatchOnly, []*testdata.Group{testdata.DoctorsGroup}, nil)
	doctorsAndNotTestersID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.SingleMessage, "resist", models.MatchOnly, []*testdata.Group{testdata.DoctorsGroup}, []*testdata.Group{testdata.TestersGroup})
	doctorsCatchallID := testdata.InsertCatchallTrigger(db, testdata.Org1, testdata.SingleMessage, []*testdata.Group{testdata.DoctorsGroup}, nil)
	othersAllID := testdata.InsertCatchallTrigger(db, testdata.Org1, testdata.SingleMessage, nil, nil)

	// trigger for other org
	testdata.InsertCatchallTrigger(db, testdata.Org2, testdata.Org2Favorites, nil, nil)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	testdata.DoctorsGroup.Add(db, testdata.Bob)
	testdata.TestersGroup.Add(db, testdata.Bob)

	_, cathy := testdata.Cathy.Load(db, oa)
	_, george := testdata.George.Load(db, oa)
	_, bob := testdata.Bob.Load(db, oa)

	tcs := []struct {
		text              string
		contact           *flows.Contact
		expectedTriggerID models.TriggerID
	}{
		{" join ", cathy, joinID},
		{"JOIN", cathy, joinID},
		{"join this", cathy, joinID},
		{"resist", george, resistID},
		{"resist", bob, doctorsID},
		{"resist", cathy, doctorsAndNotTestersID},
		{"resist this", cathy, doctorsCatchallID},
		{" 👍 ", george, emojiID},
		{"👍🏾", george, emojiID}, // is 👍 + 🏾
		{"😀👍", george, othersAllID},
		{"other", cathy, doctorsCatchallID},
		{"other", george, othersAllID},
		{"", george, othersAllID},
	}

	for _, tc := range tcs {
		trigger := models.FindMatchingMsgTrigger(oa, tc.contact, tc.text)

		assertTrigger(t, tc.expectedTriggerID, trigger, "trigger mismatch for %s sending '%s'", tc.contact.Name(), tc.text)
	}
}

func TestFindMatchingIncomingCallTrigger(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	doctorsAndNotTestersTriggerID := testdata.InsertIncomingCallTrigger(db, testdata.Org1, testdata.Favorites, []*testdata.Group{testdata.DoctorsGroup}, []*testdata.Group{testdata.TestersGroup})
	doctorsTriggerID := testdata.InsertIncomingCallTrigger(db, testdata.Org1, testdata.Favorites, []*testdata.Group{testdata.DoctorsGroup}, nil)
	notTestersTriggerID := testdata.InsertIncomingCallTrigger(db, testdata.Org1, testdata.Favorites, nil, []*testdata.Group{testdata.TestersGroup})
	everyoneTriggerID := testdata.InsertIncomingCallTrigger(db, testdata.Org1, testdata.Favorites, nil, nil)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	testdata.DoctorsGroup.Add(db, testdata.Bob)
	testdata.TestersGroup.Add(db, testdata.Bob, testdata.Alexandria)

	_, cathy := testdata.Cathy.Load(db, oa)
	_, bob := testdata.Bob.Load(db, oa)
	_, george := testdata.George.Load(db, oa)
	_, alexa := testdata.Alexandria.Load(db, oa)

	tcs := []struct {
		contact           *flows.Contact
		expectedTriggerID models.TriggerID
	}{
		{cathy, doctorsAndNotTestersTriggerID}, // they're in doctors and not in testers
		{bob, doctorsTriggerID},                // they're in doctors and testers
		{george, notTestersTriggerID},          // they're not in doctors and not in testers
		{alexa, everyoneTriggerID},             // they're not in doctors but are in testers
	}

	for _, tc := range tcs {
		trigger := models.FindMatchingIncomingCallTrigger(oa, tc.contact)

		assertTrigger(t, tc.expectedTriggerID, trigger, "trigger mismatch for %s", tc.contact.Name())
	}
}

func TestFindMatchingMissedCallTrigger(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	testdata.InsertCatchallTrigger(db, testdata.Org1, testdata.SingleMessage, nil, nil)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	// no missed call trigger yet
	trigger := models.FindMatchingMissedCallTrigger(oa)
	assert.Nil(t, trigger)

	triggerID := testdata.InsertMissedCallTrigger(db, testdata.Org1, testdata.Favorites)

	oa, err = models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	trigger = models.FindMatchingMissedCallTrigger(oa)
	assertTrigger(t, triggerID, trigger)
}

func TestFindMatchingNewConversationTrigger(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	twilioTriggerID := testdata.InsertNewConversationTrigger(db, testdata.Org1, testdata.Favorites, testdata.TwilioChannel)
	noChTriggerID := testdata.InsertNewConversationTrigger(db, testdata.Org1, testdata.Favorites, nil)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	tcs := []struct {
		channelID         models.ChannelID
		expectedTriggerID models.TriggerID
	}{
		{testdata.TwilioChannel.ID, twilioTriggerID},
		{testdata.VonageChannel.ID, noChTriggerID},
	}

	for i, tc := range tcs {
		channel := oa.ChannelByID(tc.channelID)
		trigger := models.FindMatchingNewConversationTrigger(oa, channel)

		assertTrigger(t, tc.expectedTriggerID, trigger, "trigger mismatch in test case #%d", i)
	}
}

func TestFindMatchingReferralTrigger(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	fooID := testdata.InsertReferralTrigger(db, testdata.Org1, testdata.Favorites, "foo", testdata.TwitterChannel)
	barID := testdata.InsertReferralTrigger(db, testdata.Org1, testdata.Favorites, "bar", nil)
	bazID := testdata.InsertReferralTrigger(db, testdata.Org1, testdata.Favorites, "", testdata.TwitterChannel)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	tcs := []struct {
		referrerID        string
		channelID         models.ChannelID
		expectedTriggerID models.TriggerID
	}{
		{"", testdata.TwilioChannel.ID, models.NilTriggerID},
		{"foo", testdata.TwilioChannel.ID, models.NilTriggerID},
		{"foo", testdata.TwitterChannel.ID, fooID},
		{"FOO", testdata.TwitterChannel.ID, fooID},
		{"bar", testdata.TwilioChannel.ID, barID},
		{"bar", testdata.TwitterChannel.ID, barID},
		{"zap", testdata.TwilioChannel.ID, models.NilTriggerID},
		{"zap", testdata.TwitterChannel.ID, bazID},
	}

	for i, tc := range tcs {
		channel := oa.ChannelByID(tc.channelID)
		trigger := models.FindMatchingReferralTrigger(oa, channel, tc.referrerID)

		assertTrigger(t, tc.expectedTriggerID, trigger, "trigger mismatch in test case #%d", i)
	}
}

func TestArchiveContactTriggers(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	everybodyID := testdata.InsertKeywordTrigger(db, testdata.Org1, testdata.Favorites, "join", models.MatchFirst, nil, nil)
	cathyOnly1ID := testdata.InsertScheduledTrigger(db, testdata.Org1, testdata.Favorites, nil, nil, []*testdata.Contact{testdata.Cathy})
	cathyOnly2ID := testdata.InsertScheduledTrigger(db, testdata.Org1, testdata.Favorites, nil, nil, []*testdata.Contact{testdata.Cathy})
	cathyAndGeorgeID := testdata.InsertScheduledTrigger(db, testdata.Org1, testdata.Favorites, nil, nil, []*testdata.Contact{testdata.Cathy, testdata.George})
	cathyAndGroupID := testdata.InsertScheduledTrigger(db, testdata.Org1, testdata.Favorites, []*testdata.Group{testdata.DoctorsGroup}, nil, []*testdata.Contact{testdata.Cathy})
	georgeOnlyID := testdata.InsertScheduledTrigger(db, testdata.Org1, testdata.Favorites, nil, nil, []*testdata.Contact{testdata.George})

	err := models.ArchiveContactTriggers(ctx, db, []models.ContactID{testdata.Cathy.ID, testdata.Bob.ID})
	require.NoError(t, err)

	assertTriggerArchived := func(id models.TriggerID, archived bool) {
		var isArchived bool
		db.Get(&isArchived, `SELECT is_archived FROM triggers_trigger WHERE id = $1`, id)
		assert.Equal(t, archived, isArchived, `is_archived mismatch for trigger %d`, id)
	}

	assertTriggerArchived(everybodyID, false)
	assertTriggerArchived(cathyOnly1ID, true)
	assertTriggerArchived(cathyOnly2ID, true)
	assertTriggerArchived(cathyAndGeorgeID, false)
	assertTriggerArchived(cathyAndGroupID, false)
	assertTriggerArchived(georgeOnlyID, false)
}

func TestTriggerMatchingWithSameScore(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetAll)

	db.MustExec(`DELETE FROM triggers_trigger`)

	// Create first trigger
	olderTriggerID := testdata.InsertKeywordTrigger(
		db, testdata.Org1, testdata.Favorites,
		"hello", models.MatchFirst,
		[]*testdata.Group{testdata.DoctorsGroup},
		nil,
	)

	// Force a delay to ensure different created_on timestamps
	time.Sleep(time.Millisecond * 100)

	// Create second trigger with same matching criteria (will have same score)
	newerTriggerID := testdata.InsertKeywordTrigger(
		db, testdata.Org1, testdata.SingleMessage,
		"hello", models.MatchFirst,
		[]*testdata.Group{testdata.DoctorsGroup},
		nil,
	)

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshTriggers)
	require.NoError(t, err)

	// Add test contact to doctors group
	testdata.DoctorsGroup.Add(db, testdata.Cathy)
	_, cathy := testdata.Cathy.Load(db, oa)

	// Test cases
	tcs := []struct {
		text              string
		contact           *flows.Contact
		expectedTriggerID models.TriggerID
	}{
		// Should match the newer trigger since both have same score
		{"hello", cathy, newerTriggerID},
		{"HELLO", cathy, newerTriggerID},
		{"hello world", cathy, newerTriggerID},
	}

	for _, tc := range tcs {
		trigger := models.FindMatchingMsgTrigger(oa, tc.contact, tc.text)
		assertTrigger(t, tc.expectedTriggerID, trigger,
			"trigger mismatch for %s sending '%s' - expected newer trigger with same score",
			tc.contact.Name(), tc.text)
	}

	// Get the created triggers to verify their dates
	var olderCreatedOn, newerCreatedOn time.Time
	err = db.Get(&olderCreatedOn, `SELECT created_on FROM triggers_trigger WHERE id = $1`, olderTriggerID)
	require.NoError(t, err)
	err = db.Get(&newerCreatedOn, `SELECT created_on FROM triggers_trigger WHERE id = $1`, newerTriggerID)
	require.NoError(t, err)

	// Verify that the triggers have different creation dates
	assert.True(t, olderCreatedOn.Before(newerCreatedOn),
		"expected first trigger (%s) to be created before second trigger (%s)",
		olderCreatedOn, newerCreatedOn)
}

func assertTrigger(t *testing.T, expected models.TriggerID, actual *models.Trigger, msgAndArgs ...interface{}) {
	if actual == nil {
		assert.Equal(t, expected, models.NilTriggerID, msgAndArgs...)
	} else {
		assert.Equal(t, expected, actual.ID(), msgAndArgs...)
	}
}
