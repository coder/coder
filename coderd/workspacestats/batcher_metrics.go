package workspacestats

import (
	"github.com/prometheus/client_golang/prometheus"
)

// batcherMetrics collects Prometheus metrics for the stats batcher.
type batcherMetrics struct {
	SessionCountsFoldedTotal prometheus.Counter
}

func newBatcherMetrics() batcherMetrics {
	return batcherMetrics{
		SessionCountsFoldedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentstats",
			Name:      "session_counts_folded_total",
			Help:      "Total number of reported session count entries folded into the unknown app after exceeding the per-report cap.",
		}),
	}
}

func (m batcherMetrics) register(reg prometheus.Registerer) {
	if reg != nil {
		reg.MustRegister(m.SessionCountsFoldedTotal)
	}
}
