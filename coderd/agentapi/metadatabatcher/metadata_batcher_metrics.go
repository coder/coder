package metadatabatcher

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	batchUtilization prometheus.Histogram
	droppedKeysTotal prometheus.Counter
	metadataTotal    prometheus.Counter
	publishErrors    prometheus.Counter
	batchesTotal     *prometheus.CounterVec
	batchSize        prometheus.Histogram
	flushDuration    *prometheus.HistogramVec
}

func NewMetrics() Metrics {
	return Metrics{
		batchUtilization: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_utilization",
			Help:      "Number of metadata keys per agent in each flushed batch.",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 40, 80, 160},
		}),

		droppedKeysTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_dropped_keys_total",
			Help:      "Total number of metadata keys dropped due to capacity limits.",
		}),

		batchesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batches_total",
			Help:      "Total number of metadata batches flushed.",
		}, []string{"reason"}),

		metadataTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_flushed_total",
			Help:      "Total number of unique metadatas flushed.",
		}),

		publishErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_publish_errors_total",
			Help:      "Total number of metadata batch pubsub publish calls that have resulted in an error.",
		}),

		batchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_size",
			Help:      "Total number of metadata entries in each flushed batch.",
			Buckets:   []float64{10, 25, 50, 100, 150, 200, 250, 300, 350, 400, 450, 500},
		}),

		flushDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_flush_duration_seconds",
			Help:      "Time taken to flush metadata batch to database and pubsub.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		}, []string{"reason"}),
	}
}

func (m Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.batchUtilization,
		m.droppedKeysTotal,
		m.batchesTotal,
		m.metadataTotal,
		m.batchSize,
		m.flushDuration,
	}
}

func (m Metrics) register(reg prometheus.Registerer) {
	if reg != nil {
		reg.MustRegister(m.batchUtilization)
		reg.MustRegister(m.droppedKeysTotal)
		reg.MustRegister(m.batchesTotal)
		reg.MustRegister(m.metadataTotal)
		reg.MustRegister(m.batchSize)
		reg.MustRegister(m.flushDuration)
		reg.MustRegister(m.publishErrors)
	}
}
