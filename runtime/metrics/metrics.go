package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func SetAvailableWorkers(workerQueue string, count int) {
	AvailableWorkers.WithLabelValues(workerQueue).Set(float64(count))
}

func AddWorker(workerQueue string) {
	UsedWorkers.WithLabelValues(workerQueue).Inc()
	AvailableWorkers.WithLabelValues(workerQueue).Dec()
}

func RemoveWorker(workerQueue string) {
	UsedWorkers.WithLabelValues(workerQueue).Dec()
	AvailableWorkers.WithLabelValues(workerQueue).Inc()
}

var UsedWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_used_workers",
	Help: "The number of workers currently in use",
}, []string{"queue"})

var AvailableWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_available_workers",
	Help: "The number of workers currently available",
}, []string{"queue"})
