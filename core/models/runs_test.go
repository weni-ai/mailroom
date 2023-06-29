package models_test

import (
	"testing"
	"time"

	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/goflow/flows/triggers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/runner"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuns(t *testing.T) {
	ctx, rt, db, _ := testsuite.Get()
	defer testsuite.Reset(testsuite.ResetAll)
	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(12345))

	oa, err := models.GetOrgAssetsWithRefresh(ctx, rt, testdata.Org1.ID, models.RefreshFlows)
	assert.NoError(t, err)

	// test session returns
	session := insertTestSession(t, ctx, rt, testdata.Org1, testdata.Cathy, testdata.Favorites)
	assert.Equal(t, models.SessionID(1), session.ID())
	assert.Equal(t, flows.SessionUUID("1ae96956-4b34-433e-8d1a-f05fe6923d6d"), session.UUID())
	assert.Equal(t, flows.ContactUUID("6393abc0-283d-4c9b-a1b3-641a035c34bf"), session.ContactUUID())
	assert.Equal(t, models.FlowType("M"), session.SessionType())
	assert.Equal(t, true, session.Responded())
	assert.Equal(t, "", session.Output())
	assert.Equal(t, models.OrgID(1), session.OrgID())
	assert.Equal(t, time.Now().Day(), session.CreatedOn().Day())
	assert.Nil(t, session.EndedOn())
	assert.Nil(t, session.TimeoutOn())
	assert.Nil(t, session.WaitStartedOn())
	assert.Equal(t, models.FlowID(0), session.CurrentFlowID())
	assert.Nil(t, session.ConnectionID())
	assert.Equal(t, 0, len(session.Runs()))
	assert.Nil(t, session.Sprint())
	session.ClearTimeoutOn()
	sessionScene := session.Scene()
	assert.NotNil(t, sessionScene)

	// test NewSession, FlowSession, WriteUpdatedSession
	eng := engine.NewBuilder().Build()
	flowRef := assets.NewFlowReference(testdata.SingleMessage.UUID, "Test")

	_, cathy := testdata.Cathy.Load(db, oa)

	trigger := triggers.NewBuilder(oa.Env(), flowRef, cathy).Manual().Build()
	fs, sprint, err := eng.NewSession(oa.SessionAssets(), trigger)
	require.NoError(t, err)
	tx, err := rt.DB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	sess, err := models.NewSession(ctx, tx, oa, fs, sprint)
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)
	scene := models.NewSceneForSession(sess)
	tx, err = rt.DB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	err = models.HandleEvents(ctx, rt, tx, oa, scene, sprint.Events())
	require.NoError(t, err)
	err = models.ApplyEventPreCommitHooks(ctx, rt, tx, oa, []*models.Scene{scene})
	require.NoError(t, err)
	err = tx.Commit()
	require.NoError(t, err)

	rt.Config.DB = "postgres://mailroom_test:temba@localhost/mailroom_test?sslmode=disable&Timezone=UTC"

	contactIDs := []models.ContactID{testdata.Cathy.ID}
	start := models.NewFlowStart(models.OrgID(1), models.StartTypeManual, models.FlowTypeMessaging, testdata.SingleMessage.ID, true, true).
		WithContactIDs(contactIDs)
	batch := start.CreateBatch(contactIDs, true, len(contactIDs))

	sessions, err := runner.StartFlowBatch(ctx, rt, batch)
	assert.NoError(t, err)

	csession := sessions[0]

	fcs, err := csession.FlowSession(rt.Config, oa.SessionAssets(), oa.Env())
	assert.NoError(t, err)

	tx, err = rt.DB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	err = csession.WriteUpdatedSession(ctx, rt, tx, oa, fcs, sprint, nil)
	assert.NoError(t, err)
	tx.Commit()
}
