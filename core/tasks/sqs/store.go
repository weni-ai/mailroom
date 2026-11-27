package sqs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/pkg/errors"
)

// Redis layout of the sqs_publish queue. A message is always in exactly one of these places:
//
//	sqs_publish:<org>               zset  eligible messages of an org, score = queued_on (seconds), member = task JSON
//	sqs_publish:active              zset  orgs with eligible or leased messages, score = number of leases of the org
//	sqs_publish:delayed             zset  retries waiting for their turn, score = next attempt (epoch ms, Redis clock)
//	sqs_publish:inflight            zset  leased messages, member = lease token, score = lease deadline (epoch ms, Redis clock)
//	sqs_publish:inflight:payload    hash  lease token -> task JSON, used to recover a message whose worker died
//
// Every transition is a single Lua script, so a crash can never leave a message in two places or in none. A leased
// message changes state only for the actor whose ZREM of its token returns 1 (worker confirming, worker rescheduling
// or the recovery pass), which makes those three mutually exclusive across instances. Deadlines and due times use
// the Redis clock so instances with different clocks agree on when a lease expired.
//
// Scripts check their guard first and run the writes that can fail (ZADD) before the removals: Redis does not roll
// back a script that errors half way, so a failure must happen before anything was changed.
var (
	keyActive        = fmt.Sprintf("%s:active", queue.SqsPublish)
	keyDelayed       = fmt.Sprintf("%s:delayed", queue.SqsPublish)
	keyInflight      = fmt.Sprintf("%s:inflight", queue.SqsPublish)
	keyInflightValue = fmt.Sprintf("%s:inflight:payload", queue.SqsPublish)
)

func orgKey(orgID int) string {
	return fmt.Sprintf("%s:%d", queue.SqsPublish, orgID)
}

// queueScore is the score of an eligible message: its original queued_on in seconds with microsecond resolution,
// the same format queue.AddTask uses. It never changes for a message, which is what keeps retries in place.
func queueScore(queuedOn time.Time) string {
	return strconv.FormatFloat(float64(queuedOn.UnixNano()/int64(time.Microsecond))/float64(1000000), 'f', 6, 64)
}

// scriptEnqueue adds a message to its org queue and makes sure the org is discoverable, atomically.
var scriptEnqueue = redis.NewScript(2, `-- KEYS: [OrgQueue] [Active]  ARGV: [Score] [Task] [OrgID]
	redis.call("zadd", KEYS[1], ARGV[1], ARGV[2])
	redis.call("zincrby", KEYS[2], 0, ARGV[3])
	return 1
`)

// scriptPop takes the oldest eligible message of the org with the fewest leases and leases it.
var scriptPop = redis.NewScript(3, `-- KEYS: [Active] [Inflight] [InflightPayload]  ARGV: [Token] [TTLMs] [QueuePrefix]
	local group = redis.call("zrange", KEYS[1], 0, 0)[1]
	if not group then
		return {"empty", ""}
	end

	local orgQueue = ARGV[3] .. ":" .. group
	local task = redis.call("zrange", orgQueue, 0, 0)[1]
	if not task then
		-- nothing eligible for this org, stop offering it
		redis.call("zrem", KEYS[1], group)
		return {"retry", ""}
	end

	local t = redis.call("time")
	local deadline = t[1] * 1000 + math.floor(t[2] / 1000) + tonumber(ARGV[2])

	redis.call("zadd", KEYS[2], deadline, ARGV[1])
	redis.call("hset", KEYS[3], ARGV[1], task)
	redis.call("zrem", orgQueue, task)
	redis.call("zincrby", KEYS[1], 1, group)
	return {group, task}
`)

// scriptAck ends a lease for good (sent or given up). Returns 0 when the lease was no longer ours.
var scriptAck = redis.NewScript(3, `-- KEYS: [Inflight] [InflightPayload] [Active]  ARGV: [Token] [OrgID]
	if redis.call("zrem", KEYS[1], ARGV[1]) == 0 then
		return 0
	end
	redis.call("hdel", KEYS[2], ARGV[1])

	local active = tonumber(redis.call("zincrby", KEYS[3], -1, ARGV[2]))
	if active < 0 then
		redis.call("zadd", KEYS[3], 0, ARGV[2])
	end
	return 1
`)

// scriptReschedule moves a leased message to the delayed set with its next attempt. Returns {1, next attempt epoch ms},
// or {0, 0} when the lease was no longer ours.
var scriptReschedule = redis.NewScript(4, `-- KEYS: [Inflight] [InflightPayload] [Delayed] [Active]  ARGV: [Token] [OrgID] [Task] [DelayMs]
	if not redis.call("zscore", KEYS[1], ARGV[1]) then
		return {0, 0}
	end

	local t = redis.call("time")
	local at = t[1] * 1000 + math.floor(t[2] / 1000) + tonumber(ARGV[4])

	redis.call("zadd", KEYS[3], at, ARGV[3])
	redis.call("zrem", KEYS[1], ARGV[1])
	redis.call("hdel", KEYS[2], ARGV[1])

	local active = tonumber(redis.call("zincrby", KEYS[4], -1, ARGV[2]))
	if active < 0 then
		redis.call("zadd", KEYS[4], 0, ARGV[2])
	end
	return {1, at}
`)

// scriptPromote returns a due retry to its org queue at its original position. Returns 0 when somebody else did.
var scriptPromote = redis.NewScript(3, `-- KEYS: [Delayed] [Active] [OrgQueue]  ARGV: [Task] [OrgID] [Score]
	if not redis.call("zscore", KEYS[1], ARGV[1]) then
		return 0
	end

	redis.call("zadd", KEYS[3], ARGV[3], ARGV[1])
	redis.call("zincrby", KEYS[2], 0, ARGV[2])
	redis.call("zrem", KEYS[1], ARGV[1])
	return 1
`)

// scriptReclaim takes over a lease whose deadline passed. With a new task it goes back to its org queue, with an
// empty one it is dropped (attempts exhausted). Returns 0 when the lease is gone or has not expired.
var scriptReclaim = redis.NewScript(4, `-- KEYS: [Inflight] [InflightPayload] [Active] [OrgQueue]  ARGV: [Token] [OrgID] [Task or ""] [Score]
	local deadline = redis.call("zscore", KEYS[1], ARGV[1])
	if not deadline then
		return 0
	end

	local t = redis.call("time")
	local now = t[1] * 1000 + math.floor(t[2] / 1000)
	if tonumber(deadline) > now then
		return 0
	end

	if ARGV[3] ~= "" then
		redis.call("zadd", KEYS[4], ARGV[4], ARGV[3])
	end
	redis.call("zrem", KEYS[1], ARGV[1])
	redis.call("hdel", KEYS[2], ARGV[1])

	-- the decrement also leaves the org listed in active, so a requeued message is found again
	local active = tonumber(redis.call("zincrby", KEYS[3], -1, ARGV[2]))
	if active < 0 then
		redis.call("zadd", KEYS[3], 0, ARGV[2])
	end
	return 1
`)

// newLeaseToken returns an unguessable, unique token for a lease. It is not derived from the task content, so
// two identical tasks never share a lease.
func newLeaseToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, "error generating lease token")
	}
	return hex.EncodeToString(b), nil
}

// enqueue stores a new message and makes its org discoverable in one atomic step. QueuedOn must already be set.
func enqueue(rc redis.Conn, task *queue.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = scriptEnqueue.Do(rc, orgKey(task.OrgID), keyActive, queueScore(task.QueuedOn), data, task.OrgID)
	return err
}

// pop leases the next message. It returns nil when nothing is eligible. The lease lasts `ttl` on the Redis clock.
func pop(rc redis.Conn, token string, ttl time.Duration) (*queue.Task, error) {
	for {
		values, err := redis.Strings(scriptPop.Do(rc, keyActive, keyInflight, keyInflightValue, token, ttl.Milliseconds(), queue.SqsPublish))
		if err != nil {
			return nil, err
		}

		switch values[0] {
		case "empty":
			return nil, nil
		case "retry":
			continue
		}

		task := &queue.Task{}
		if err := json.Unmarshal([]byte(values[1]), task); err != nil {
			// the lease stays and is dropped by the recovery pass, an unreadable task can never be sent
			return nil, errors.Wrap(err, "error unmarshalling leased sqs task")
		}
		task.LeaseToken = token
		return task, nil
	}
}

// ack ends the lease of a message that was sent or given up. It reports false when the lease was no longer ours.
func ack(rc redis.Conn, orgID int, token string) (bool, error) {
	n, err := redis.Int(scriptAck.Do(rc, keyInflight, keyInflightValue, keyActive, token, orgID))
	return n == 1, err
}

// reschedule ends the lease and stores `next` as a retry due after `delay`. It reports the time of the next attempt
// on the Redis clock, or false when the lease was no longer ours.
func reschedule(rc redis.Conn, orgID int, token string, next *queue.Task, delay time.Duration) (time.Time, bool, error) {
	data, err := json.Marshal(next)
	if err != nil {
		return time.Time{}, false, err
	}
	res, err := redis.Int64s(scriptReschedule.Do(rc, keyInflight, keyInflightValue, keyDelayed, keyActive, token, orgID, data, delay.Milliseconds()))
	if err != nil {
		return time.Time{}, false, err
	}
	if res[0] != 1 {
		return time.Time{}, false, nil
	}
	return time.UnixMilli(res[1]), true, nil
}

// redisNowMs returns the Redis clock in epoch milliseconds
func redisNowMs(rc redis.Conn) (int64, error) {
	parts, err := redis.Strings(rc.Do("time"))
	if err != nil || len(parts) != 2 {
		return 0, errors.Wrap(err, "error reading redis time")
	}
	sec, _ := strconv.ParseInt(parts[0], 10, 64)
	usec, _ := strconv.ParseInt(parts[1], 10, 64)
	return sec*1000 + usec/1000, nil
}

// promoteDue returns up to `batch` retries whose time has come to their org queues, at their original position.
func promoteDue(rc redis.Conn, batch int) (int, error) {
	now, err := redisNowMs(rc)
	if err != nil {
		return 0, err
	}

	members, err := redis.ByteSlices(rc.Do("zrangebyscore", keyDelayed, "-inf", now, "limit", 0, batch))
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, member := range members {
		task := &queue.Task{}
		if err := json.Unmarshal(member, task); err != nil {
			// an unreadable retry can never be sent, drop it so it does not block the batch forever
			rc.Do("zrem", keyDelayed, member)
			logReclaimDropped(0, errors.Wrap(err, "unreadable delayed task"))
			continue
		}

		n, err := redis.Int(scriptPromote.Do(rc, keyDelayed, keyActive, orgKey(task.OrgID), member, task.OrgID, queueScore(task.QueuedOn)))
		if err != nil {
			return promoted, err
		}
		promoted += n
	}
	return promoted, nil
}

// reclaimExpired takes over up to `batch` leases whose deadline passed. Each goes back to its org queue with one more
// attempt, or is given up when that would exceed maxAttempts. It returns the messages put back.
func reclaimExpired(rc redis.Conn, maxAttempts, batch int) ([]recoveredItem, error) {
	now, err := redisNowMs(rc)
	if err != nil {
		return nil, err
	}

	tokens, err := redis.Strings(rc.Do("zrangebyscore", keyInflight, "-inf", now, "limit", 0, batch))
	if err != nil {
		return nil, err
	}

	var recovered []recoveredItem
	for _, token := range tokens {
		raw, err := redis.Bytes(rc.Do("hget", keyInflightValue, token))
		if err != nil && err != redis.ErrNil {
			return recovered, err
		}

		task := &queue.Task{}
		if err == redis.ErrNil || json.Unmarshal(raw, task) != nil {
			// without a readable task there is no org to credit and nothing to resend
			rc.Send("zrem", keyInflight, token)
			rc.Send("hdel", keyInflightValue, token)
			rc.Do("")
			logReclaimDropped(0, errors.New("leased task is missing or unreadable"))
			continue
		}

		p := &PublishTask{}
		if err := json.Unmarshal(task.Task, p); err != nil {
			// poison task: drop it, resending would fail the same way
			if _, aerr := ack(rc, task.OrgID, token); aerr != nil {
				return recovered, aerr
			}
			logReclaimDropped(task.OrgID, errors.Wrap(err, "unreadable leased task"))
			continue
		}

		info := newDeliveryInfo(task, p)

		// a recovered lease counts as an attempt, so a task that crashes its worker every time still ends
		next := *p
		next.Attempt = p.Attempt + 1

		newTask := ""
		if next.Attempt <= maxAttempts {
			nextTask := *task
			nextTask.LeaseToken = ""
			nextTask.Task, err = json.Marshal(&next)
			if err != nil {
				return recovered, err
			}
			data, err := json.Marshal(&nextTask)
			if err != nil {
				return recovered, err
			}
			newTask = string(data)
		}

		n, err := redis.Int(scriptReclaim.Do(rc, keyInflight, keyInflightValue, keyActive, orgKey(task.OrgID), token, task.OrgID, newTask, queueScore(task.QueuedOn)))
		if err != nil {
			return recovered, err
		}
		if n != 1 {
			continue // somebody else got there first, or the lease was extended
		}

		if newTask == "" {
			logDeadLettered(info, "lease expired and attempts exhausted", time.Time{})
			continue
		}
		recovered = append(recovered, recoveredItem{TicketUUID: info.TicketUUID, CorrelationID: info.CorrelationID, OrgID: info.OrgID, QueueURL: info.QueueURL})
	}
	return recovered, nil
}
