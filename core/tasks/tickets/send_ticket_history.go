package tickets

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
)

func init() {
	mailroom.AddTaskFunction(queue.SendHistory, handleSendTicketHistory)
}

// SendHistoryTask represents a task to send ticket history
type SendHistoryTask struct {
	TicketUUID flows.TicketUUID `json:"ticket_uuid"`
	ContactID  models.ContactID `json:"contact_id"`
}

// HandleSendTicketHistory processes the send ticket history task (exported for testing)
func HandleSendTicketHistory(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
	return handleSendTicketHistory(ctx, rt, task)
}

// handleSendTicketHistory processes the send ticket history task
func handleSendTicketHistory(ctx context.Context, rt *runtime.Runtime, task *queue.Task) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*10)
	defer cancel()

	// decode our task body
	if task.Type != queue.SendHistory {
		return errors.Errorf("unknown event type passed to send history worker: %s", task.Type)
	}

	sendHistoryTask := &SendHistoryTask{}
	err := json.Unmarshal(task.Task, sendHistoryTask)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling send history task: %s", string(task.Task))
	}

	return SendTicketHistory(ctx, rt, sendHistoryTask)
}

// SendTicketHistory executes the ticket history sending logic
func SendTicketHistory(ctx context.Context, rt *runtime.Runtime, task *SendHistoryTask) error {
	// Load the ticket
	ticket, err := models.LookupTicketByUUID(ctx, rt.DB, task.TicketUUID)
	if err != nil {
		return errors.Wrapf(err, "error loading ticket with uuid %s", task.TicketUUID)
	}

	// Get org assets
	oa, err := models.GetOrgAssets(ctx, rt, ticket.OrgID())
	if err != nil {
		return errors.Wrapf(err, "error getting org assets")
	}

	// Get the ticketer for this ticket
	ticketer := oa.TicketerByID(ticket.TicketerID())
	if ticketer == nil {
		return errors.Errorf("can't find ticketer with id %d", ticket.TicketerID())
	}

	// Create the service
	service, err := ticketer.AsService(rt.Config, flows.NewTicketer(ticketer))
	if err != nil {
		return errors.Wrapf(err, "error creating ticketer service")
	}

	// Create HTTP logger for the ticketer service calls
	logger := &models.HTTPLogger{}

	// Send the history - most implementations don't actually use the runs parameter
	// or only use it to get the start time, so we can pass nil here
	err = service.SendHistory(ticket, task.ContactID, nil, logger.Ticketer(ticketer))

	// Insert HTTP logs regardless of success/failure
	logger.Insert(ctx, rt.DB)

	if err != nil {
		return errors.Wrapf(err, "error sending ticket history")
	}

	return nil
}

// QueueSendHistory queues a task to send ticket history
func QueueSendHistory(rc redis.Conn, ticket *models.Ticket, contactID models.ContactID) error {
	task := &SendHistoryTask{
		TicketUUID: ticket.UUID(),
		ContactID:  contactID,
	}

	return queue.AddTask(rc, queue.HandlerQueue, queue.SendHistory, int(ticket.OrgID()), task, queue.DefaultPriority)
}
