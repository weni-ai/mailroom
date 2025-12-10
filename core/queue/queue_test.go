package queue

import (
	"encoding/json"
	"testing"
	"time"
	"strconv"

	"github.com/gomodule/redigo/redis"
	"github.com/stretchr/testify/assert"
)

func TestQueues(t *testing.T) {
	rc, err := redis.Dial("tcp", "localhost:6379")
	assert.NoError(t, err)
	rc.Do("del", "test:active", "test:1", "test:2", "test:3")

	popPriority := Priority(-1)
	markCompletePriority := Priority(-2)

	tcs := []struct {
		Queue     string
		TaskGroup int
		TaskType  string
		Task      string
		Priority  Priority
		Size      int
	}{
		{"test", 1, "campaign", "task1", DefaultPriority, 1},
		{"test", 1, "campaign", "task1", popPriority, 0},
		{"test", 1, "campaign", "", popPriority, 0},
		{"test", 1, "campaign", "task1", DefaultPriority, 1},
		{"test", 1, "campaign", "task2", DefaultPriority, 2},
		{"test", 2, "campaign", "task3", DefaultPriority, 3},
		{"test", 2, "campaign", "task4", DefaultPriority, 4},
		{"test", 1, "campaign", "task5", DefaultPriority, 5},
		{"test", 2, "campaign", "task6", DefaultPriority, 6},
		{"test", 1, "campaign", "task1", popPriority, 5},
		{"test", 2, "campaign", "task3", popPriority, 4},
		{"test", 1, "campaign", "task2", popPriority, 3},
		{"test", 2, "campaign", "task4", popPriority, 2},
		{"test", 2, "campaign", "", markCompletePriority, 2},
		{"test", 2, "campaign", "task6", popPriority, 1},
		{"test", 1, "campaign", "task5", popPriority, 0},
		{"test", 1, "campaign", "", popPriority, 0},
	}

	for i, tc := range tcs {
		if tc.Priority == popPriority {
			task, err := PopNextTask(rc, "test")

			if task == nil {
				if tc.Task != "" {
					assert.Fail(t, "%d: did not receive task, expected %s", i, tc.Task)
				}
				continue
			} else if tc.Task == "" && task != nil {
				assert.Fail(t, "%d: received task %s when expecting none", i, tc.Task)
				continue
			}

			assert.NoError(t, err)
			assert.Equal(t, task.OrgID, tc.TaskGroup, "%d: groups mismatch", i)
			assert.Equal(t, task.Type, tc.TaskType, "%d: types mismatch", i)

			var value string
			assert.NoError(t, json.Unmarshal(task.Task, &value), "%d: error unmarshalling", i)
			assert.Equal(t, value, tc.Task, "%d: task mismatch", i)
		} else if tc.Priority == markCompletePriority {
			assert.NoError(t, MarkTaskComplete(rc, tc.Queue, tc.TaskGroup))
		} else {
			assert.NoError(t, AddTask(rc, tc.Queue, tc.TaskType, tc.TaskGroup, tc.Task, tc.Priority))
		}

		size, err := Size(rc, tc.Queue)
		assert.NoError(t, err)
		assert.Equal(t, tc.Size, size, "%d: mismatch", i)
	}
}

func TestProcessingAndRequeueSendHistory(t *testing.T) {
	rc, err := redis.Dial("tcp", "localhost:6379")
	assert.NoError(t, err)
	defer rc.Close()

	// use isolated queue and org
	q := "qproc"
	org := 1234
	// cleanup keys
	rc.Do("del", q+":active", q+":payload:*", q+":*")

	// mark org as active to be discovered by RequeueExpired
	_, err = rc.Do("zincrby", q+":active", 1, org)
	assert.NoError(t, err)

	// create a send_history task
	task := &Task{
		Type:  SendHistory,
		OrgID: org,
		Task:  json.RawMessage(`{"ticket_uuid":"u","contact_id":1}`),
	}

	// begin processing with already expired TTL
	taskKey, err := BeginProcessing(rc, q, org, task, -1*time.Second)
	assert.NoError(t, err)
	assert.NotEmpty(t, taskKey)

	// ensure it's in processing
	count, err := redis.Int(rc.Do("zcard", q+":"+strconvI(org)+":processing"))
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// run requeue
	n, err := RequeueExpired(rc, q, time.Now())
	assert.NoError(t, err)
	assert.Equal(t, 1, n)

	// verify moved back to main queue with ErrorCount incremented
	values, err := redis.ByteSlices(rc.Do("zrange", q+":"+strconvI(org), 0, -1))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(values))

	var t2 Task
	assert.NoError(t, json.Unmarshal(values[0], &t2))
	assert.Equal(t, SendHistory, t2.Type)
	assert.Equal(t, org, t2.OrgID)
	assert.Equal(t, 1, t2.ErrorCount)

	// processing set empty and payload deleted
	count, err = redis.Int(rc.Do("zcard", q+":"+strconvI(org)+":processing"))
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	// active reset to 0 for org
	score, err := redis.Float64(rc.Do("zscore", q+":active", org))
	assert.NoError(t, err)
	assert.Equal(t, 0.0, score)
}

// helper to format int consistently for redis commands that accept interface{}
func strconvI(i int) string {
	return strconv.Itoa(i)
}
