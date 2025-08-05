package msgs

import (
	"context"
	"encoding/json"
	"slices"

	"time"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/msgio"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func init() {
	mailroom.AddTaskFunction(queue.SendWppBroadcast, handleSendWppBroadcast)
	mailroom.AddTaskFunction(queue.SendWppBroadcastBatch, handleSendWppBroadcastBatch)
}

func handleSendWppBroadcast(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*60)
	defer cancel()

	// decode our task body
	if task.Type != queue.SendWppBroadcast {
		return errors.Errorf("unknown event type passed to send worker: %s", task.Type)
	}
	broadcast := &models.WppBroadcast{}
	err := json.Unmarshal(task.Task, broadcast)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling broadcast: %s", string(task.Task))
	}

	return CreateWppBroadcastBatches(ctx, rt, broadcast)
}

func CreateWppBroadcastBatches(ctx context.Context, rt *runtime.Runtime, bcast *models.WppBroadcast) error {
	// we are building a set of contact ids, start with the explicit ones
	contactIDs := make(map[models.ContactID]bool)
	for _, id := range bcast.ContactIDs() {
		contactIDs[id] = true
	}

	groupContactIDs, err := models.ContactIDsForGroupIDs(ctx, rt.DB, bcast.GroupIDs())
	if err != nil {
		return errors.Wrapf(err, "error getting contact ids for group ids")
	}

	for _, id := range groupContactIDs {
		contactIDs[id] = true
	}

	oa, err := models.GetOrgAssets(ctx, rt, bcast.OrgID())
	if err != nil {
		return errors.Wrapf(err, "error getting org assets")
	}

	// get the contact ids for our URNs
	urnMap, err := models.GetOrCreateContactIDsFromURNs(ctx, rt.DB, oa, bcast.URNs())
	if err != nil {
		return errors.Wrapf(err, "error getting contact ids for urns")
	}

	urnContacts := make(map[models.ContactID]urns.URN)
	repeatedContacts := make(map[models.ContactID]urns.URN)

	q := queue.WppBroadcastBatchQueue
	priority := queue.DefaultPriority

	if bcast.Queue() != "" {
		// check if the queue is valid
		allowedQueues := []string{queue.WppBroadcastBatchQueue, queue.TemplateBatchQueue, queue.TemplateNotificationBatchQueue}
		if !slices.Contains(allowedQueues, bcast.Queue()) {
			return errors.Errorf("invalid queue for wpp broadcast: %s", bcast.Queue())
		}

		q = bcast.Queue()

		// if we are on the template batch or template notification batch queue, we want to use low priority
		lowPriorityQueues := []string{queue.TemplateBatchQueue, queue.TemplateNotificationBatchQueue}
		if slices.Contains(lowPriorityQueues, bcast.Queue()) {
			priority = queue.LowPriority
		}
	}

	// we want to remove contacts that are also present in URN sends, these will be a special case in our last batch
	for u, id := range urnMap {
		if contactIDs[id] {
			repeatedContacts[id] = u
			delete(contactIDs, id) // if more than one contact contact with different urns, may cause fail if urn no exists, so this have to be handled previously
		}
		urnContacts[id] = u
	}

	rc := rt.RP.Get()
	defer rc.Close()

	contacts := make([]models.ContactID, 0, 100)

	// utility functions for queueing the current set of contacts
	queueBatch := func(isLast bool) {
		// if this is our last batch include those contacts that overlap with our urns
		if isLast {
			for id := range repeatedContacts {
				contacts = append(contacts, id)
			}
		}

		batch := bcast.CreateBatch(contacts)

		// also set our URNs
		if isLast {
			batch.SetIsLast(true)
			batch.SetURNs(urnContacts)
		}

		err = queue.AddTask(rc, q, queue.SendWppBroadcastBatch, int(bcast.OrgID()), batch, priority)
		if err != nil {
			logrus.WithError(err).Error("error while queuing wpp broadcast batch")
		}
		contacts = make([]models.ContactID, 0, 100)
	}

	// build up batches of contacts to start
	for c := range contactIDs {
		if len(contacts) == startBatchSize {
			queueBatch(false)
		}
		contacts = append(contacts, c)
	}

	// queue our last batch
	queueBatch(true)

	return nil
}

func handleSendWppBroadcastBatch(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*60)
	defer cancel()

	// decode our task body
	if task.Type != queue.SendWppBroadcastBatch {
		return errors.Errorf("unknown event type passed to send worker: %s", task.Type)
	}
	broadcast := &models.WppBroadcastBatch{}
	err := json.Unmarshal(task.Task, broadcast)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling broadcast: %s", string(task.Task))
	}

	// try to send the batch
	return SendWppBroadcastBatch(ctx, rt, broadcast)
}

func SendWppBroadcastBatch(ctx context.Context, rt *runtime.Runtime, bcast *models.WppBroadcastBatch) error {
	// always set our broadcast as sent if it is our last
	defer func() {
		if bcast.IsLast() {
			err := models.MarkBroadcastSent(ctx, rt.DB, bcast.BroadcastID())
			if err != nil {
				logrus.WithError(err).Error("error marking broadcast as sent")
			}
		}
	}()

	oa, err := models.GetOrgAssets(ctx, rt, bcast.OrgID())
	if err != nil {
		return errors.Wrapf(err, "error getting org assets")
	}

	// create this batch of messages
	msgs, err := models.CreateWppBroadcastMessages(ctx, rt, oa, bcast)
	if err != nil {
		return errors.Wrapf(err, "error creating broadcast messages")
	}

	msgio.SendMessages(ctx, rt, rt.DB, nil, msgs)
	return nil
}
