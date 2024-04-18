package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var usedWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_used_workers",
	Help: "The number of workers currently in use",
}, []string{"queue"})

var availableWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_available_workers",
	Help: "The number of workers currently available",
}, []string{"queue"})

func SetAvailableWorkers(workerQueue string, count int) {
	availableWorkers.WithLabelValues(workerQueue).Set(float64(count))
}

func SetUsedWorkers(workerQueue string, count int) {
	usedWorkers.WithLabelValues(workerQueue).Set(float64(count))
}
