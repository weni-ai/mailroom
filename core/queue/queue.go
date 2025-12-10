package queue

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
)

// Task is a utility struct for encoding a task
type Task struct {
	Type       string          `json:"type"`
	OrgID      int             `json:"org_id"`
	Task       json.RawMessage `json:"task"`
	QueuedOn   time.Time       `json:"queued_on"`
	ErrorCount int             `json:"error_count,omitempty"`
}

// Priority is the priority for the task
type Priority int

const (
	queuePattern  = "%s:%d"
	activePattern = "%s:active"

	// DefaultPriority is the default priority for tasks
	DefaultPriority = Priority(0)

	// HighPriority is the highest priority for tasks
	HighPriority = Priority(-10000000)

	// LowPriority is the lowest priority for tasks
	LowPriority = Priority(+10000000)

	// BatchQueue is our queue for batch tasks, most things that operate on more than one cotact at a time
	BatchQueue = "batch"

	// HandlerQueue is our queue for message handling or other tasks related to just one contact
	HandlerQueue = "handler"

	// FlowBatchQueue is our queue for flow batch tasks
	FlowBatchQueue = "flow_batch"

	// WppBroadcastBatchQueue is our queue for wpp broadcast batch tasks primarily used by nexus application and general uses
	WppBroadcastBatchQueue = "wpp_broadcast_batch"

	// TemplateBatchQueue is our queue for template batch tasks primarily used on templates campaigns
	TemplateBatchQueue = "template_batch"

	// TemplateNotificationBatchQueue is our queue for template notification batch tasks primarily used by commerce application
	TemplateNotificationBatchQueue = "template_notification_batch"

	// SendBroadcast is our type for sending a broadcast
	SendBroadcast = "send_broadcast"

	// SendBroadcastBatch is our type for sending a broadcast batch
	SendBroadcastBatch = "send_broadcast_batch"

	// SendTemplateBroadcast is our type for sending a template broadcast
	SendWppBroadcast = "send_whatsapp_broadcast"

	// SendTemplateBroadcastBatch is our type for sending a template broadcast batch
	SendWppBroadcastBatch = "send_whatsapp_broadcast_batch"

	// HandleContactEvent is our task for event handling
	HandleContactEvent = "handle_contact_event"

	// StartFlow is our task type to start a flow
	StartFlow = "start_flow"

	// StartFlowBatch is our task for starting a flow batch
	StartFlowBatch = "start_flow_batch"

	// StartIVRFlowBatch is our task for starting an ivr batch
	StartIVRFlowBatch = "start_ivr_flow_batch"

	// SendHistory is our task for sending history to a ticket integration
	SendHistory = "send_history"

	// RabbitmqPublish is our task type for publishing a message to RabbitMQ
	RabbitmqPublish = "rabbitmq_publish"
)

// Size returns the number of tasks for the passed in queue
func Size(rc redis.Conn, queue string) (int, error) {
	// get all the active queues
	queues, err := redis.Ints(rc.Do("zrange", fmt.Sprintf(activePattern, queue), 0, -1))
	if err != nil {
		return 0, errors.Wrapf(err, "error getting active queues for: %s", queue)
	}

	// add up each
	size := 0
	for _, q := range queues {
		count, err := redis.Int(rc.Do("zcard", fmt.Sprintf(queuePattern, queue, q)))
		if err != nil {
			return 0, errors.Wrapf(err, "error getting size of: %d", q)
		}
		size += count
	}

	return size, nil
}

// AddTask adds the passed in task to our queue for execution
func AddTask(rc redis.Conn, queue string, taskType string, orgID int, task interface{}, priority Priority) error {
	score := strconv.FormatFloat(float64(time.Now().UnixNano()/int64(time.Microsecond))/float64(1000000)+float64(priority), 'f', 6, 64)

	taskBody, err := json.Marshal(task)
	if err != nil {
		return err
	}

	payload := &Task{
		Type:     taskType,
		OrgID:    orgID,
		Task:     taskBody,
		QueuedOn: time.Now(),
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	rc.Send("zadd", fmt.Sprintf(queuePattern, queue, orgID), score, jsonPayload)
	rc.Send("zincrby", fmt.Sprintf(activePattern, queue), 0, orgID)
	_, err = rc.Do("")
	return err
}

var popTask = redis.NewScript(1, `-- KEYS: [QueueName]
    -- first get what is the active queue
	local result = redis.call("zrange", KEYS[1] .. ":active", 0, 0, "WITHSCORES")

	-- nothing? return nothing
	local group = result[1]
	if not group then
		return {"empty", ""}
	end

	local queue = KEYS[1] .. ":" .. group

	-- pop off our queue
	local result = redis.call("zrangebyscore", queue, 0, "+inf", "WITHSCORES", "LIMIT", 0, 1)

	-- found a result?
	if result[1] then
		-- then remove it from the queue
		redis.call('zremrangebyrank', queue, 0, 0)

		-- and add a worker to this queue
		redis.call("zincrby", KEYS[1] .. ":active", 1, group)

		return {group, result[1]}
	else
		-- no result found, remove this group from active queues
		redis.call("zrem", KEYS[1] .. ":active", group)

		return {"retry", ""}
	end
`)

// PopNextTask pops the next task off our queue
func PopNextTask(rc redis.Conn, queue string) (*Task, error) {
	task := Task{}
	for {
		values, err := redis.Strings(popTask.Do(rc, queue))
		if err != nil {
			return nil, err
		}

		if values[0] == "empty" {
			return nil, nil
		}

		if values[0] == "retry" {
			continue
		}

		err = json.Unmarshal([]byte(values[1]), &task)
		return &task, err
	}
}

var markComplete = redis.NewScript(2, `-- KEYS: [QueueName] [TaskGroup]
	-- decrement our active
	local active = tonumber(redis.call("zincrby", KEYS[1] .. ":active", -1, KEYS[2]))

	-- reset to zero if we somehow go below
	if active < 0 then
		redis.call("zadd", KEYS[1] .. ":active", 0, KEYS[2])
	end
`)

// MarkTaskComplete marks the passed in task as complete. Callers must call this in order
// to maintain fair workers across orgs
func MarkTaskComplete(rc redis.Conn, queue string, orgID int) error {
	_, err := markComplete.Do(rc, queue, strconv.FormatInt(int64(orgID), 10))
	return err
}

// processingZSet returns the processing zset name for an org in a queue
func processingZSet(queue string, orgID int) string {
	return fmt.Sprintf("%s:%d:processing", queue, orgID)
}

// payloadKey returns the key to store a processing task payload
func payloadKey(queue string, taskKey string) string {
	return fmt.Sprintf("%s:payload:%s", queue, taskKey)
}

// taskFingerprint generates a deterministic fingerprint for a task payload
func taskFingerprint(t *Task) string {
	h := sha1.New()
	h.Write([]byte(t.Type))
	h.Write([]byte{0})
	h.Write([]byte(fmt.Sprintf("%d", t.OrgID)))
	h.Write([]byte{0})
	h.Write([]byte(t.Task))
	return hex.EncodeToString(h.Sum(nil))
}

// BeginProcessing registers a task as being processed with a deadline and stores its payload
func BeginProcessing(rc redis.Conn, queue string, orgID int, task *Task, ttl time.Duration) (string, error) {
	taskKey := taskFingerprint(task)
	deadline := time.Now().Add(ttl).UnixMilli()

	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}

	_, err = rc.Do("zadd", processingZSet(queue, orgID), "NX", deadline, taskKey)
	if err != nil {
		return "", err
	}
	_, err = rc.Do("set", payloadKey(queue, taskKey), payload)
	if err != nil {
		rc.Do("zrem", processingZSet(queue, orgID), taskKey)
		return "", err
	}
	return taskKey, nil
}

// EndProcessing removes a task from processing and deletes its stored payload
func EndProcessing(rc redis.Conn, queue string, orgID int, taskKey string) error {
	if taskKey == "" {
		return nil
	}
	rc.Send("zrem", processingZSet(queue, orgID), taskKey)
	rc.Send("del", payloadKey(queue, taskKey))
	_, err := rc.Do("")
	return err
}

// AddTaskAt adds a task back to a queue with an explicit scheduled time
func AddTaskAt(rc redis.Conn, queue string, orgID int, payload *Task, at time.Time, priority Priority) error {
	score := fmt.Sprintf("%.6f", float64(at.UnixNano()/int64(time.Microsecond))/float64(1000000)+float64(priority))
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rc.Send("zadd", fmt.Sprintf(queuePattern, queue, orgID), score, jsonPayload)
	rc.Send("zincrby", fmt.Sprintf(activePattern, queue), 0, orgID)
	_, err = rc.Do("")
	return err
}

// RequeueExpired looks for processing tasks past deadline and requeues them with backoff
func RequeueExpired(rc redis.Conn, queue string, now time.Time) (int, error) {
	orgIDs, err := redis.Ints(rc.Do("zrange", fmt.Sprintf(activePattern, queue), 0, -1))
	if err != nil {
		return 0, err
	}

	requeued := 0
	for _, orgID := range orgIDs {
		expiredKeys, err := redis.Strings(rc.Do("zrangebyscore", processingZSet(queue, orgID), "-inf", now.UnixMilli()))
		if err != nil {
			return requeued, err
		}
		for _, key := range expiredKeys {
			payloadJSON, err := redis.Bytes(rc.Do("get", payloadKey(queue, key)))
			if err != nil || len(payloadJSON) == 0 {
				rc.Do("zrem", processingZSet(queue, orgID), key)
				rc.Do("del", payloadKey(queue, key))
				continue
			}

			var t Task
			if err := json.Unmarshal(payloadJSON, &t); err != nil {
				rc.Do("zrem", processingZSet(queue, orgID), key)
				rc.Do("del", payloadKey(queue, key))
				continue
			}

			t.ErrorCount++
			base := time.Second
			max := 5 * time.Minute
			delay := base * time.Duration(1<<uint(t.ErrorCount-1))
			if delay > max {
				delay = max
			}

			when := now.Add(delay)
			if err := AddTaskAt(rc, queue, t.OrgID, &t, when, DefaultPriority); err == nil {
				requeued++
			}

			rc.Do("zincrby", fmt.Sprintf(activePattern, queue), -1, orgID)
			rc.Do("zrem", processingZSet(queue, orgID), key)
			rc.Do("del", payloadKey(queue, key))
		}
	}

	return requeued, nil
}
