package mailroom

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/mailroom/runtime/metrics"

	"github.com/gomodule/redigo/redis"
	"github.com/sirupsen/logrus"
)

// TaskSource is where a foreman gets the tasks of its queue and how it hands them back once handled. Most queues use
// the default source. A queue whose tasks must survive a crash while being handled (sqs_publish) registers its own
// with RegisterTaskSource, because it leases the task on pop and the task function confirms it, so the foreman must
// not mark the task complete itself.
type TaskSource interface {
	// Pop returns the next task of the queue, or nil if there is none
	Pop(rt *runtime.Runtime, rc redis.Conn) (*queue.Task, error)

	// Release is called when a worker is done with a popped task, whether it succeeded, failed or panicked
	Release(rt *runtime.Runtime, rc redis.Conn, task *queue.Task) error
}

// defaultTaskSource pops with queue.PopNextTask and marks the task complete to keep workers fair across orgs
type defaultTaskSource struct {
	queue string
}

func (s defaultTaskSource) Pop(rt *runtime.Runtime, rc redis.Conn) (*queue.Task, error) {
	return queue.PopNextTask(rc, s.queue)
}

func (s defaultTaskSource) Release(rt *runtime.Runtime, rc redis.Conn, task *queue.Task) error {
	return queue.MarkTaskComplete(rc, s.queue, task.OrgID)
}

// taskLog adds the raw task body to a log entry, except for queues whose bodies carry personal data (the sqs_publish
// body has the contact URN), which must never reach logs or Sentry.
func taskLog(log *logrus.Entry, task *queue.Task) *logrus.Entry {
	if task.Type == queue.SqsPublish {
		return log
	}
	return log.WithField("task", string(task.Task))
}

// Foreman takes care of managing our set of workers and assigns msgs for each to send
type Foreman struct {
	rt               *runtime.Runtime
	wg               *sync.WaitGroup
	queue            string
	source           TaskSource
	workers          []*Worker
	availableWorkers chan *Worker
	quit             chan bool
}

// PendingTaskInfo describes a task that is currently being processed by a worker
type PendingTaskInfo struct {
	Queue    string
	WorkerID int
	TaskType string
	OrgID    int
}

// NewForeman creates a new Foreman for the passed in server with the number of max workers
func NewForeman(rt *runtime.Runtime, wg *sync.WaitGroup, queue string, maxWorkers int) *Foreman {
	foreman := &Foreman{
		rt:               rt,
		wg:               wg,
		queue:            queue,
		source:           taskSourceFor(queue),
		workers:          make([]*Worker, maxWorkers),
		availableWorkers: make(chan *Worker, maxWorkers),
		quit:             make(chan bool),
	}

	for i := 0; i < maxWorkers; i++ {
		foreman.workers[i] = NewWorker(foreman, i)
	}

	return foreman
}

// Start starts the foreman and all its workers, assigning jobs while there are some
func (f *Foreman) Start() {
	metrics.SetAvailableWorkers(f.queue, len(f.workers))
	metrics.SetUsedWorkers(f.queue, 0)

	for _, worker := range f.workers {
		worker.Start()
	}
	go f.Assign()

	// start a background reaper to requeue expired processing tasks
	if f.queue == queue.HandlerQueue {
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			log := logrus.WithField("comp", "foreman_reaper").WithField("queue", f.queue)
			for {
				select {
				case <-f.quit:
					log.WithField("state", "stopped").Info("reaper stopped")
					return
				case <-ticker.C:
					rc := f.rt.RP.Get()
					_, err := queue.RequeueExpired(rc, f.queue, time.Now())
					rc.Close()
					if err != nil {
						log.WithError(err).Error("error requeuing expired tasks")
					}
				}
			}
		}()
	}
}

// Stop stops the foreman and all its workers, the wait group of the worker can be used to track progress
func (f *Foreman) Stop() {
	for _, worker := range f.workers {
		worker.Stop()
	}
	close(f.quit)
	logrus.WithField("comp", "foreman").WithField("queue", f.queue).WithField("state", "stopping").Info("foreman stopping")

	metrics.SetAvailableWorkers(f.queue, 0)
	metrics.SetUsedWorkers(f.queue, 0)
}

// Assign is our main loop for the Foreman, it takes care of popping the next outgoing task from our
// backend and assigning them to workers
func (f *Foreman) Assign() {
	f.wg.Add(1)
	defer f.wg.Done()
	log := logrus.WithField("comp", "foreman").WithField("queue", f.queue)

	log.WithFields(logrus.Fields{
		"state":   "started",
		"workers": len(f.workers),
		"queue":   f.queue,
	}).Info("workers started and waiting")

	lastSleep := false

	go f.RecordWorkerMetrics()

	for {
		select {
		// return if we have been told to stop
		case <-f.quit:
			log.WithField("state", "stopped").Info("foreman stopped")
			return

		// otherwise, grab the next task and assign it to a worker
		case worker := <-f.availableWorkers:
			// see if we have a task to work on
			rc := f.rt.RP.Get()
			task, err := f.source.Pop(f.rt, rc)
			rc.Close()

			if err == nil && task != nil {
				// if so, assign it to our worker
				worker.job <- task
				lastSleep = false
			} else {
				// we received an error getting the next message, log it
				if err != nil {
					log.WithError(err).Error("error popping task")
				}

				// add our worker back to our queue and sleep a bit
				if !lastSleep {
					log.Debug("sleeping, no tasks")
					lastSleep = true
				}
				f.availableWorkers <- worker
				time.Sleep(250 * time.Millisecond)
			}
		}
	}
}

func (f *Foreman) RecordWorkerMetrics() {
	for {
		metrics.SetAvailableWorkers(f.queue, len(f.availableWorkers))
		metrics.SetUsedWorkers(f.queue, len(f.workers)-len(f.availableWorkers))
		time.Sleep(250 * time.Millisecond)
	}
}

// PendingTasks returns a snapshot of all tasks that are currently being processed by this foreman's workers.
// This is intended for debugging and observability (for example, when handling shutdown signals).
func (f *Foreman) PendingTasks() []PendingTaskInfo {
	pending := make([]PendingTaskInfo, 0)

	for _, w := range f.workers {
		if task, ok := w.snapshotCurrentTask(); ok && task != nil {
			pending = append(pending, PendingTaskInfo{
				Queue:    f.queue,
				WorkerID: w.id,
				TaskType: task.Type,
				OrgID:    task.OrgID,
			})
		}
	}

	return pending
}

// Worker is our type for a single goroutine that is handling queued events
type Worker struct {
	id      int
	foreman *Foreman
	job     chan *queue.Task

	mu          sync.RWMutex
	currentTask *queue.Task
}

// NewWorker creates a new worker responsible for working on events
func NewWorker(foreman *Foreman, id int) *Worker {
	worker := &Worker{
		id:      id,
		foreman: foreman,
		job:     make(chan *queue.Task, 1),
	}
	return worker
}

// Start starts our Worker's goroutine and has it start waiting for tasks from the foreman
func (w *Worker) Start() {
	w.foreman.wg.Add(1)

	go func() {
		defer w.foreman.wg.Done()

		log := logrus.WithField("queue", w.foreman.queue).WithField("worker_id", w.id)
		log.Debug("started")

		for {
			// list ourselves as available for work
			w.foreman.availableWorkers <- w

			// grab our next piece of work
			task := <-w.job

			// exit if we were stopped
			if task == nil {
				w.clearCurrentTask()
				log.Debug("stopped")
				return
			}

			w.setCurrentTask(task)
			w.handleTask(task)
		}
	}()
}

// Stop stops our worker
func (w *Worker) Stop() {
	close(w.job)
}

func (w *Worker) handleTask(task *queue.Task) {
	log := logrus.WithField("queue", w.foreman.queue).WithField("worker_id", w.id).WithField("task_type", task.Type).WithField("org_id", task.OrgID)

	// register taskKey in outer scope so our defer can access it
	var taskKey string

	defer func() {
		// catch any panics and recover
		panicLog := recover()
		if panicLog != nil {
			debug.PrintStack()
			taskLog(log, task).WithField("task_type", task.Type).WithField("org_id", task.OrgID).Errorf("panic handling task: %s", panicLog)
		}

		// clear our current task snapshot
		w.clearCurrentTask()

		// mark our task as complete
		rc := w.foreman.rt.RP.Get()
		if task.Type == queue.SendHistory && taskKey != "" {
			_ = queue.EndProcessing(rc, w.foreman.queue, task.OrgID, taskKey)
		}
		err := w.foreman.source.Release(w.foreman.rt, rc, task)
		if err != nil {
			log.WithError(err).Error("error releasing task")
		}
		rc.Close()
	}()

	// register task as processing with a deadline
	{
		if task.Type == queue.SendHistory {
			rc := w.foreman.rt.RP.Get()
			processingTTL := time.Duration(w.foreman.rt.Config.ProcessingTTL) * time.Second
			key, err := queue.BeginProcessing(rc, w.foreman.queue, task.OrgID, task, processingTTL)
			rc.Close()
			if err == nil {
				taskKey = key
			} else {
				log.WithError(err).Warn("unable to register task as processing")
			}
		}
	}

	log.Info("starting handling of task")
	start := time.Now()

	taskFunc, found := taskFunctions[task.Type]
	if found {
		err := taskFunc(context.Background(), w.foreman.rt, task)
		if err != nil {
			taskLog(log, task).WithError(err).Error("error running task")
		}
	} else {
		log.Error("unable to find function for task type")
	}

	elapsed := time.Since(start)

	log.WithField("elapsed", elapsed).Info("task complete")

	// additionally if any task took longer than 1 minute, log as warning
	if elapsed > time.Minute {
		taskLog(log, task).WithField("elapsed", elapsed).Warn("long running task")
	}
}

// setCurrentTask records the task currently being processed by this worker.
func (w *Worker) setCurrentTask(task *queue.Task) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.currentTask = task
}

// clearCurrentTask clears any record of a task being processed by this worker.
func (w *Worker) clearCurrentTask() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.currentTask = nil
}

// snapshotCurrentTask returns a snapshot of the current task, if any, for observability.
func (w *Worker) snapshotCurrentTask() (*queue.Task, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.currentTask == nil {
		return nil, false
	}
	return w.currentTask, true
}
