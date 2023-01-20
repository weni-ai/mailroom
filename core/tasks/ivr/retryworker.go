package ivr

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/nyaruka/mailroom/core/ivr"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
)

type Job struct {
	Id   int
	conn *models.ChannelConnection
}

type JobResult struct {
	Output string
}

func handleWork(id int, rt *runtime.Runtime, wg *sync.WaitGroup, jobChannel <-chan Job) {
	defer wg.Done()
	lastExecutionTime := time.Now()
	minimumTimeBetweenEachExecution := time.Duration(math.Ceil(1e9 / float64(rt.Config.IVRRetryMaximumExecutionsPerSecond)))

	for job := range jobChannel {
		timeUntilNextExecution := -(time.Since(lastExecutionTime) - minimumTimeBetweenEachExecution)
		if timeUntilNextExecution > 0 {
			fmt.Printf("Worker #%d backing off for %s\n", id, timeUntilNextExecution)
			time.Sleep(timeUntilNextExecution)
		} else {
			fmt.Printf("Worker #%d not backing off \n", id)
		}
		lastExecutionTime = time.Now()
		retryCall(id, rt, job.conn)
	}
}

func retryCall(workerId int, rt *runtime.Runtime, conn *models.ChannelConnection) JobResult {
	log := logrus.WithField("connection_id", conn.ID())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	oa, err := models.GetOrgAssets(ctx, rt, conn.OrgID())
	if err != nil {
		log.WithError(err).WithField("org_id", conn.OrgID()).Error("erroro loading org")
		return JobResult{Output: "Fail"}
	}

	channel := oa.ChannelByID(conn.ChannelID())
	if channel == nil {
		err = models.UpdateChannelConnectionStatuses(ctx, rt.DB, []models.ConnectionID{conn.ID()}, models.ConnectionStatusFailed)
		if err != nil {
			log.WithError(err).WithField("channel_id", conn.ChannelID()).Error("error marking call as failed due to missing channel")
		}
		return JobResult{Output: "Fail"}
	}

	urn, err := models.URNForID(ctx, rt.DB, oa, conn.ContactURNID())
	if err != nil {
		log.WithError(err).WithField("urn_id", conn.ContactURNID()).Error("unable to load contact urn")
		return JobResult{Output: "Fail"}
	}

	err = ivr.RequestCallStartForConnection(ctx, rt, channel, urn, conn)
	if err != nil {
		log.WithError(err).Error(err)
		return JobResult{Output: "Fail"}
	}

	return JobResult{Output: "Success"}
}
