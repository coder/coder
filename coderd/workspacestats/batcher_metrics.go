package workspacestats

import (
	"github.com/prometheus/client_golang/prometheus"
)

// batcherMetrics collects Prometheus metrics for the stats batcher.
type batcherMetrics struct {
	SessionCountsOverflowTotal prometheus.Counter
}

func newBatcherMetrics() batcherMetrics {
	return batcherMetrics{
		SessionCountsOverflowTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentstats",
			Name:      "session_counts_overflow_total",
			Help:      "Total number of reported session count entries for unregistered apps that were summed into an overflow app.",
		}),
	}
}

func (m batcherMetrics) register(reg prometheus.Registerer) {
	if reg != nil {
		reg.MustRegister(m.SessionCountsOverflowTotal)
	}
}
