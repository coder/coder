package alerts

import (
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	DispatchAttempts *prometheus.CounterVec
	RetryCount       *prometheus.CounterVec

	QueuedSeconds *prometheus.HistogramVec

	InflightDispatches    *prometheus.GaugeVec
	DispatcherSendSeconds *prometheus.HistogramVec

	PendingUpdates prometheus.Gauge
	SyncedUpdates  prometheus.Counter
}

const (
	ns        = "coderd"
	subsystem = "notifications"

	LabelMethod     = "method"
	LabelTemplateID = "alert_template_id"
	LabelResult     = "result"

	ResultSuccess  = "success"
	ResultTempFail = "temp_fail"
	ResultPermFail = "perm_fail"
)

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		DispatchAttempts: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "dispatch_attempts_total", Namespace: ns, Subsystem: subsystem,
			Help: fmt.Sprintf("The number of dispatch attempts, aggregated by the result type (%s)",
				strings.Join([]string{ResultSuccess, ResultTempFail, ResultPermFail}, ", ")),
		}, []string{LabelMethod, LabelTemplateID, LabelResult}),
		RetryCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "retry_count", Namespace: ns, Subsystem: subsystem,
			Help: "The count of notification dispatch retry attempts.",
		}, []string{LabelMethod, LabelTemplateID}),

		// Aggregating on LabelTemplateID as well would cause a cardinality explosion.
		QueuedSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "queued_seconds", Namespace: ns, Subsystem: subsystem,
			Buckets: []float64{1, 2.5, 5, 7.5, 10, 15, 20, 30, 60, 120, 300, 600, 3600},
			Help: "The time elapsed between a notification being enqueued in the store and retrieved for dispatching " +
				"(measures the latency of the notifications system). This should generally be within CODER_NOTIFICATIONS_FETCH_INTERVAL " +
				"seconds; higher values for a sustained period indicates delayed processing and CODER_NOTIFICATIONS_LEASE_COUNT " +
				"can be increased to accommodate this.",
		}, []string{LabelMethod}),

		InflightDispatches: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "inflight_dispatches", Namespace: ns, Subsystem: subsystem,
			Help: "The number of dispatch attempts which are currently in progress.",
		}, []string{LabelMethod, LabelTemplateID}),
		// Aggregating on LabelTemplateID as well would cause a cardinality explosion.
		DispatcherSendSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "dispatcher_send_seconds", Namespace: ns, Subsystem: subsystem,
			Buckets: []float64{0.001, 0.05, 0.1, 0.5, 1, 2, 5, 10, 15, 30, 60, 120},
			Help:    "The time taken to dispatch notifications.",
		}, []string{LabelMethod}),

		// Currently no requirement to discriminate between success and failure updates which are pending.
		PendingUpdates: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pending_updates", Namespace: ns, Subsystem: subsystem,
			Help: "The number of dispatch attempt results waiting to be flushed to the store.",
		}),
		SyncedUpdates: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "synced_updates_total", Namespace: ns, Subsystem: subsystem,
			Help: "The number of dispatch attempt results flushed to the store.",
		}),
	}
}
