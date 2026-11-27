package sqs

import (
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
)

func init() {
	mailroom.RegisterTaskSource(queue.SqsPublish, taskSource{})
}

// taskSource is how the sqs_publish foreman gets its tasks. Popping leases the task for ProcessingTTL seconds instead
// of just removing it, and the task function confirms it (ack) or reschedules it, so Release has nothing to do: the
// foreman must not mark the task complete itself, that would release the org slot twice.
type taskSource struct{}

// Pop leases the next task. The returned task carries its LeaseToken.
func (taskSource) Pop(rt *runtime.Runtime, rc redis.Conn) (*queue.Task, error) {
	token, err := newLeaseToken()
	if err != nil {
		return nil, err
	}
	return pop(rc, token, time.Duration(rt.Config.ProcessingTTL)*time.Second)
}

// Release does nothing, see the type comment
func (taskSource) Release(rt *runtime.Runtime, rc redis.Conn, task *queue.Task) error {
	return nil
}
