package metrics

import (
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func SetAvailableWorkers(workerQueue string, count int) {
	switch workerQueue {
	case queue.HandlerQueue:
		HandlerWorkersAvailable.Set(float64(count))
	case queue.BatchQueue:
		BatchWorkersAvailable.Set(float64(count))
	case queue.FlowBatchQueue:
		FlowBatchWorkersAvailable.Set(float64(count))
	}
}

func AddWorker(workerQueue string) {
	switch workerQueue {
	case queue.HandlerQueue:
		HandlerWorkersInUse.Inc()
		HandlerWorkersAvailable.Dec()
	case queue.BatchQueue:
		BatchWorkersInUse.Inc()
		BatchWorkersAvailable.Dec()
	case queue.FlowBatchQueue:
		FlowBatchWorkersInUse.Inc()
		FlowBatchWorkersAvailable.Dec()
	}
}

func RemoveWorker(workerQueue string) {
	switch workerQueue {
	case queue.HandlerQueue:
		HandlerWorkersInUse.Dec()
		HandlerWorkersAvailable.Inc()
	case queue.BatchQueue:
		BatchWorkersInUse.Dec()
		BatchWorkersAvailable.Inc()
	case queue.FlowBatchQueue:
		FlowBatchWorkersInUse.Dec()
		FlowBatchWorkersAvailable.Inc()
	}
}

var HandlerWorkersInUse = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_handler_workers_in_use",
	Help: "The number of handler workers currently in use",
})

var HandlerWorkersAvailable = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_handler_workers_available",
	Help: "The number of handler workers currently available",
})

var BatchWorkersInUse = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_batch_workers_in_use",
	Help: "The number of batch workers currently in use",
})

var BatchWorkersAvailable = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_batch_workers_available",
	Help: "The number of batch workers currently available",
})

var FlowBatchWorkersInUse = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_flow_batch_workers_in_use",
	Help: "The number of flow batch workers currently in use",
})

var FlowBatchWorkersAvailable = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "mr_flow_batch_workers_available",
	Help: "The number of flow batch workers currently available",
})
