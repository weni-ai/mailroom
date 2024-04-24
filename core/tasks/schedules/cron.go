package schedules

import (
	"context"
	"sync"
	"time"

	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/utils/cron"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	scheduleLock = "fire_schedules"
)

func init() {
	mailroom.AddInitFunction(StartCheckSchedules)
}

// StartCheckSchedules starts our cron job of firing schedules every minute
func StartCheckSchedules(rt *runtime.Runtime, wg *sync.WaitGroup, quit chan bool) error {
	cron.Start(quit, rt, scheduleLock, time.Minute*1, false,
		func() error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
			defer cancel()
			// we sleep 1 second since we fire right on the minute and want to make sure to fire
			// things that are schedules right at the minute as well (and DB time may be slightly drifted)
			time.Sleep(time.Second * 1)
			return checkSchedules(ctx, rt)
		},
	)
	return nil
}

// checkSchedules looks up any expired schedules and fires them, setting the next fire as needed
func checkSchedules(ctx context.Context, rt *runtime.Runtime) error {
	log := logrus.WithField("comp", "schedules_cron")
	start := time.Now()

	rc := rt.RP.Get()
	defer rc.Close()

	// get any expired schedules
	unfired, err := models.GetUnfiredSchedules(ctx, rt.DB)
	if err != nil {
		return errors.Wrapf(err, "error while getting unfired schedules")
	}

	// for each unfired schedule
	broadcasts := 0
	triggers := 0
	noops := 0

	for _, s := range unfired {
		log := log.WithField("schedule_id", s.ID())
		now := time.Now()

		// grab our timezone
		tz, err := s.Timezone()
		if err != nil {
			log.WithError(err).Error("error firing schedule, unknown timezone")
			continue
		}

		// calculate our next fire
		nextFire, err := s.GetNextFire(tz, now)
		if err != nil {
			log.WithError(err).Error("error calculating next fire for schedule")
			continue
		}

		// open a transaction for committing all the items for this fire
		tx, err := rt.DB.BeginTxx(ctx, nil)
		if err != nil {
			log.WithError(err).Error("error starting transaction for schedule fire")
			continue
		}

		var task interface{}
		var taskName string
		taskQueue := queue.BatchQueue

		// if it is a broadcast
		if s.Broadcast() != nil {
			// clone our broadcast, our schedule broadcast is just a template
			bcast, err := models.InsertChildBroadcast(ctx, tx, s.Broadcast())
			if err != nil {
				log.WithError(err).Error("error inserting new broadcast for schedule")
				tx.Rollback()
				continue
			}

			// add our task to send this broadcast
			task = bcast
			taskName = queue.SendBroadcast
			broadcasts++

		} else if s.FlowStart() != nil {
			start := s.FlowStart()

			// insert our flow start
			err := models.InsertFlowStarts(ctx, tx, []*models.FlowStart{start})
			if err != nil {
				log.WithError(err).Error("error inserting new flow start for schedule")
				tx.Rollback()
				continue
			}

			// add our flow start task
			task = start
			taskName = queue.StartFlow
			taskQueue = queue.FlowBatchQueue
			triggers++
		} else {
			log.Info("schedule found with no associated active broadcast or trigger, ignoring")
			noops++
		}

		// update our next fire for this schedule
		err = s.UpdateFires(ctx, tx, now, nextFire)
		if err != nil {
			log.WithError(err).Error("error updating next fire for schedule")
			tx.Rollback()
			continue
		}

		// commit our transaction
		err = tx.Commit()
		if err != nil {
			log.WithError(err).Error("error comitting schedule transaction")
			tx.Rollback()
			continue
		}

		// add our task if we have one
		if task != nil {
			err = queue.AddTask(rc, taskQueue, taskName, int(s.OrgID()), task, queue.HighPriority)
			if err != nil {
				log.WithError(err).Error("error firing task with name: ", taskName)
			}
		}
	}

	log.WithFields(logrus.Fields{
		"broadcasts": broadcasts,
		"triggers":   triggers,
		"noops":      noops,
		"elapsed":    time.Since(start),
	}).Info("fired schedules")

	return nil
}
