package metrics

import (
	"os"
	"strconv"
	"strings"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var orgsToMonitor = map[models.OrgID]bool{}
var globalLabel = "global"

func init() {
	orgsString := os.Getenv("MAILROOM_PROMETHEUS_MONITOR_ORGS")
	if orgsString != "" {
		orgs := strings.Split(orgsString, ",")
		for _, org := range orgs {
			orgId, err := strconv.ParseInt(org, 10, 64)
			if err != nil {
				logrus.Errorf("Invalid org ID %d", orgId)
				continue
			}
			orgsToMonitor[models.OrgID(orgId)] = true
		}
	}

	logrus.WithField("orgs", orgsToMonitor).Info("prometheus orgs to monitor")
}

func orgIdToString(orgId models.OrgID) string {
	return strconv.FormatInt(int64(orgId), 10)
}

var summaryObjectives = map[float64]float64{
	0.5:  0.05,  // 50th percentile with a max. absolute error of 0.05.
	0.90: 0.01,  // 90th percentile with a max. absolute error of 0.01.
	0.95: 0.005, // 95th percentile with a max. absolute error of 0.005.
	0.99: 0.001, // 99th percentile with a max. absolute error of 0.001.
}

var usedWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_used_workers",
	Help: "The number of workers currently in use",
}, []string{"queue"})

var availableWorkers = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_available_workers",
	Help: "The number of workers currently available",
}, []string{"queue"})

var tasksQueueSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_tasks_queue_size",
	Help: "The number of tasks currently in the queue directly from redis, updated every ~1 minute",
}, []string{"queue"})

var dbStats = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_db_stats",
	Help: "Database stats, updated every ~1 minute. 'db_busy' is the number of connections currently in use. 'db_idle' is the number of idle connections. 'db_waiting' is the number of connections that are being waited for. 'db_wait_ms' is the total time blocked waiting for a new connection in milliseconds",
}, []string{"stat"})

var flowStartElapsed = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_flow_start_elapsed",
	Help:       "The time a flow engine execution took to complete for a single contact",
	Objectives: summaryObjectives,
}, []string{"orgId"})

var flowBatchStartElapsed = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_flow_batch_start_elapsed",
	Help:       "The time a flow batch start took to complete for the contact batch",
	Objectives: summaryObjectives,
}, []string{"orgId"})

var flowBatchStartCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_flow_batch_start_count",
	Help: "The number of contacts that were started in a flow batch",
}, []string{"orgId"})

var campaignEventElapsed = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_campaign_event_elapsed",
	Help:       "The time a campaign event execution took to complete",
	Objectives: summaryObjectives,
}, []string{"orgId"})

var campaignEventCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_campaign_event_count",
	Help: "The number of sessions that where started due to campaign events",
}, []string{"orgId"})

var campaignEventCronElapsed = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_campaign_event_cron_elapsed",
	Help:       "The time a campaign event cron execution took to queue all batches",
	Objectives: summaryObjectives,
}, []string{"orgId"})

var campaignEventCronCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "mr_campaign_event_cron_count",
	Help: "The number of campaign events that where queued",
}, []string{"orgId"})

var contactEventElapsed = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_contact_event_elapsed",
	Help:       "The time a contact event handling took to complete in this runtime",
	Objectives: summaryObjectives,
}, []string{"orgId", "event_type"})

var contactEventLatency = promauto.NewSummaryVec(prometheus.SummaryOpts{
	Name:       "mr_contact_event_latency",
	Help:       "The time a contact event handling took to complete since it was queued",
	Objectives: summaryObjectives,
}, []string{"orgId", "event_type"})

func SetAvailableWorkers(workerQueue string, count int) {
	availableWorkers.WithLabelValues(workerQueue).Set(float64(count))
}

func SetUsedWorkers(workerQueue string, count int) {
	usedWorkers.WithLabelValues(workerQueue).Set(float64(count))
}

func SetQueueSize(queue string, size int) {
	tasksQueueSize.WithLabelValues(queue).Set(float64(size))
}

func SetDBStats(stat string, value float64) {
	dbStats.WithLabelValues(stat).Set(value)
}

func ObserveFlowStartElapsed(orgId models.OrgID, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		flowStartElapsed.WithLabelValues(orgIdToString(orgId)).Observe(elapsed)
	}
	flowStartElapsed.WithLabelValues(globalLabel).Observe(elapsed)
}

func ObserveFlowBatchStartElapsed(orgId models.OrgID, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		flowBatchStartElapsed.WithLabelValues(orgIdToString(orgId)).Observe(elapsed)
	}
	flowBatchStartElapsed.WithLabelValues(globalLabel).Observe(elapsed)
}

func AddFlowBatchStartCount(orgId models.OrgID, count float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		flowBatchStartCount.WithLabelValues(orgIdToString(orgId)).Add(count)
	}
	flowBatchStartCount.WithLabelValues(globalLabel).Add(count)
}

func ObserveCampaignEventElapsed(orgId models.OrgID, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		campaignEventElapsed.WithLabelValues(orgIdToString(orgId)).Observe(elapsed)
	}
	campaignEventElapsed.WithLabelValues(globalLabel).Observe(elapsed)
}

func AddCampaignEventCount(orgId models.OrgID, count float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		campaignEventCount.WithLabelValues(orgIdToString(orgId)).Add(count)
	}
	campaignEventCount.WithLabelValues(globalLabel).Add(count)
}

func ObserveCampaignEventCronElapsed(orgId models.OrgID, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		campaignEventCronElapsed.WithLabelValues(orgIdToString(orgId)).Observe(elapsed)
	}
	campaignEventCronElapsed.WithLabelValues(globalLabel).Observe(elapsed)
}

func AddCampaignEventCronCount(orgId models.OrgID, count float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		campaignEventCronCount.WithLabelValues(orgIdToString(orgId)).Add(count)
	}
	campaignEventCronCount.WithLabelValues(globalLabel).Add(count)
}

func ObserveContactEventElapsed(orgId models.OrgID, eventType string, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		contactEventElapsed.WithLabelValues(orgIdToString(orgId), eventType).Observe(elapsed)
	}
	contactEventElapsed.WithLabelValues(globalLabel, eventType).Observe(elapsed)
}

func ObserveContactEventLatency(orgId models.OrgID, eventType string, elapsed float64) {
	if _, ok := orgsToMonitor[orgId]; ok {
		contactEventLatency.WithLabelValues(orgIdToString(orgId), eventType).Observe(elapsed)
	}
	contactEventLatency.WithLabelValues(globalLabel, eventType).Observe(elapsed)
}
