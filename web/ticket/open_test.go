package ticket

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/sqs"
	"github.com/nyaruka/mailroom/services/tickets/wenichats"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
	"github.com/nyaruka/mailroom/web"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logTimeRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

// Q-01 (HTTP path): every ticket opened through the endpoint of a multi-agent org is queued for SQS with origin "http",
// and the enqueued log carries what is needed to find it again
func TestOpenTicketEnqueuesSQSMessageWithHTTPOrigin(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()

	defer testsuite.Reset(testsuite.ResetData)
	testsuite.Reset(testsuite.ResetRedis)

	db.MustExec(`UPDATE orgs_org SET config = '{"is_multi_agents": true}' WHERE id = $1`, testdata.Org1.ID)
	_, err := models.GetOrgAssets(ctx, rt, testdata.Org1.ID)
	require.NoError(t, err)

	hook := logtest.NewGlobal()
	defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))

	wenichats.SetDB(rt.DB)
	web.RunWebTests(t, ctx, rt, "testdata/open.json", nil)

	rc := rp.Get()
	defer rc.Close()

	members, err := redis.ByteSlices(rc.Do("zrange", "sqs_publish:1", 0, -1))
	require.NoError(t, err)
	require.NotEmpty(t, members, "tickets were opened, so messages must be queued")

	corrIDs := map[string]bool{}
	for _, member := range members {
		task := &queue.Task{}
		require.NoError(t, json.Unmarshal(member, task))

		p := &sqs.PublishTask{}
		require.NoError(t, json.Unmarshal(task.Task, p))
		require.NotNil(t, p.Meta)
		assert.Equal(t, "http", p.Meta.Origin)
		assert.NotEmpty(t, p.Meta.TicketUUID)
		assert.NotEmpty(t, p.Meta.CorrelationID)
		assert.False(t, p.Meta.CreatedOn.IsZero())
		corrIDs[p.Meta.CorrelationID] = true
	}

	var enqueued []*logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Data["event"] == "enqueued" {
			enqueued = append(enqueued, e)
		}
	}
	require.Len(t, enqueued, len(members))

	for _, e := range enqueued {
		assert.Equal(t, logrus.InfoLevel, e.Level)
		assert.NotEmpty(t, e.Data["ticket_uuid"])
		assert.True(t, corrIDs[e.Data["correlation_id"].(string)])
		assert.Equal(t, int(testdata.Org1.ID), e.Data["org_id"])
		assert.NotEmpty(t, e.Data["queue_url"])
		assert.Equal(t, "http", e.Data["origin"])
		for _, field := range []string{"created_on", "queued_on", "enqueued_at"} {
			assert.Regexp(t, logTimeRegex, e.Data[field], field)
		}
	}
}
