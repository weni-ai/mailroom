package sqs

import (
	"sync"
	"time"

	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/runtime"
)

const (
	// sqsPublishMaintenanceTick is how often the maintenance loop promotes due retries and reclaims expired leases
	sqsPublishMaintenanceTick = time.Second

	// sqsPublishMaintenanceBatch is the maximum number of items promoted or reclaimed per pass
	sqsPublishMaintenanceBatch = 100
)

func init() {
	mailroom.AddInitFunction(startMaintenance)
}

// startMaintenance recovers messages whose lease already expired, synchronously so it is done before the foremen start
// taking tasks, and then keeps promoting due retries and recovering expired leases until quit is closed.
//
// A lease only expires ProcessingTTL seconds after it was taken, so messages of a process that just crashed are
// recovered once their lease runs out, not immediately: without a stable identity per instance there is no way to
// tell them from the leases of a live instance. Every instance runs this loop; the Redis scripts keep them from
// stepping on each other.
func startMaintenance(rt *runtime.Runtime, wg *sync.WaitGroup, quit chan bool) error {
	if !rt.Config.SqsPublishEnabled || rt.SQS == nil {
		return nil
	}

	maintenancePass(rt)

	wg.Add(1)
	go func() {
		defer wg.Done()

		ticker := time.NewTicker(sqsPublishMaintenanceTick)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				maintenancePass(rt)
			}
		}
	}()
	return nil
}

// maintenancePass promotes due retries and recovers expired leases, one batch of each
func maintenancePass(rt *runtime.Runtime) {
	rc := rt.RP.Get()
	defer rc.Close()

	// a failure leaves the items where they are, the next pass tries again
	if _, err := promoteDue(rc, sqsPublishMaintenanceBatch); err != nil {
		logMaintenanceError("error promoting sqs retries", err)
	}

	recovered, err := reclaimExpired(rc, rt.Config.SqsPublishMaxAttempts, sqsPublishMaintenanceBatch)
	if err != nil {
		logMaintenanceError("error recovering expired sqs leases", err)
	}
	logRecovered(recovered)
}
