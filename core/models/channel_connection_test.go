package models_test

import (
	"testing"
	"time"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
)

func TestChannelConnections(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer db.MustExec(`DELETE FROM channels_channelconnection`)

	conn, err := models.InsertIVRConnection(ctx, db, testdata.Org1.ID, testdata.TwilioChannel.ID, models.NilStartID, testdata.Cathy.ID, testdata.Cathy.URNID, models.ConnectionDirectionOut, models.ConnectionStatusPending, "")
	assert.NoError(t, err)

	assert.NotEqual(t, models.ConnectionID(0), conn.ID())

	err = conn.UpdateExternalID(ctx, db, "test1")
	assert.NoError(t, err)

	testsuite.AssertQuery(t, db, `SELECT count(*) from channels_channelconnection where external_id = 'test1' AND id = $1`, conn.ID()).Returns(1)

	conn2, err := models.SelectChannelConnection(ctx, db, conn.ID())
	assert.NoError(t, err)
	assert.Equal(t, "test1", conn2.ExternalID())
	assert.Equal(t, testdata.Org1.ID, conn2.OrgID())
	assert.Equal(t, models.ConnectionStatus("W"), conn2.Status())
	assert.Equal(t, models.ConnectionError(""), conn2.ErrorReason())
	assert.Equal(t, 0, conn2.ErrorCount())
	assert.Equal(t, models.StartID(0), conn2.StartID())
	var noMoment *time.Time = nil
	assert.Equal(t, noMoment, conn2.NextAttempt())

	conn3, err := models.SelectChannelConnectionByExternalID(ctx, db, testdata.TwilioChannel.ID, models.ConnectionTypeIVR, "test1")
	assert.NoError(t, err)
	assert.Equal(t, "test1", conn3.ExternalID())

	connCount, err := models.ActiveChannelConnectionCount(ctx, db, testdata.TwilioChannel.ID)
	assert.NoError(t, err)
	assert.Equal(t, 1, connCount)

	err = conn3.MarkStarted(ctx, db, time.Now())
	assert.NoError(t, err)

	err = conn3.MarkThrottled(ctx, db, time.Now())
	assert.NoError(t, err)

	err = conn3.UpdateStatus(ctx, db, models.ConnectionStatusQueued, 1, time.Now())
	assert.NoError(t, err)

	// next time attempt will be in 10 milliseconds
	mockDuration := time.Millisecond * 10
	err = conn3.MarkErrored(ctx, db, time.Now(), &mockDuration, models.ConnectionError(models.ConnectionStatusErrored))
	assert.NoError(t, err)

	// after 10 milliseconds test1 connection will be available to be retried
	time.Sleep(100 * time.Millisecond)
	conns, err := models.LoadChannelConnectionsToRetry(ctx, db, 10)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(conns))

	err = models.UpdateChannelConnectionStatuses(ctx, db, []models.ConnectionID{conn3.ID()}, models.ConnectionStatusErrored)
	assert.NoError(t, err)

	err = conn3.MarkFailed(ctx, db, time.Now())
	assert.NoError(t, err)
}
