package notifications

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	DispatchedCount  *prometheus.CounterVec
	TempFailureCount *prometheus.CounterVec
	PermFailureCount *prometheus.CounterVec
	RetryCount       *prometheus.CounterVec

	QueuedSeconds *prometheus.HistogramVec

	DispatcherSendSeconds *prometheus.HistogramVec

	PendingUpdates prometheus.Gauge
}

const (
	ns        = "coderd"
	subsystem = "notifications"

	LabelMethod     = "method"
	LabelTemplateID = "template_id"
)

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		DispatchedCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "dispatched_count", Namespace: ns, Subsystem: subsystem,
			Help: "The count of notifications successfully dispatched.",
		}, []string{LabelMethod, LabelTemplateID}),
		TempFailureCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "temporary_failures_count", Namespace: ns, Subsystem: subsystem,
			Help: "The count of notifications which failed but have retry attempts remaining.",
		}, []string{LabelMethod, LabelTemplateID}),
		PermFailureCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "permanent_failures_count", Namespace: ns, Subsystem: subsystem,
			Help: "The count of notifications which failed and have exceeded their retry attempts.",
		}, []string{LabelMethod, LabelTemplateID}),
		RetryCount: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "retry_count", Namespace: ns, Subsystem: subsystem,
			Help: "The count of notification dispatch retry attempts.",
		}, []string{LabelMethod, LabelTemplateID}),

		// Aggregating on LabelTemplateID as well would cause a cardinality explosion.
		QueuedSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "queued_seconds", Namespace: ns, Subsystem: subsystem,
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 3600, 86400},
			Help: "The time elapsed between a notification being enqueued in the store and retrieved for processing " +
				"(measures the latency of the notifications system). This should generally be within CODER_NOTIFICATIONS_FETCH_INTERVAL " +
				"seconds; higher values for a sustained period indicates delayed processing and CODER_NOTIFICATIONS_LEASE_COUNT " +
				"can be increased to accommodate this.",
		}, []string{LabelMethod}),

		// Aggregating on LabelTemplateID as well would cause a cardinality explosion.
		DispatcherSendSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "dispatcher_send_seconds", Namespace: ns, Subsystem: subsystem,
			Buckets: []float64{0.001, 0.05, 0.1, 0.5, 1, 2, 5, 10, 15, 30, 60, 120},
			Help:    "The time taken to dispatch notifications.",
		}, []string{LabelMethod}),

		// Currently no requirement to discriminate between success and failure updates which are pending.
		PendingUpdates: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Name: "pending_updates", Namespace: ns, Subsystem: subsystem,
			Help: "The number of updates waiting to be flushed to the store.",
		}),
	}
}
