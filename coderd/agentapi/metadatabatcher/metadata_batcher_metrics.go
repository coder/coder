package metadatabatcher

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	BatchUtilization prometheus.Histogram
	FlushDuration    *prometheus.HistogramVec
	BatchSize        prometheus.Histogram
	BatchesTotal     *prometheus.CounterVec
	DroppedKeysTotal prometheus.Counter
	MetadataTotal    prometheus.Counter
	PublishErrors    prometheus.Counter
}

func NewMetrics() Metrics {
	return Metrics{
		BatchUtilization: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_utilization",
			Help:      "Number of metadata keys per agent in each batch, updated before flushes.",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 40, 80, 160},
		}),

		BatchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_size",
			Help:      "Total number of metadata entries in each batch, updated before flushes.",
			Buckets:   []float64{10, 25, 50, 100, 150, 200, 250, 300, 350, 400, 450, 500},
		}),

		FlushDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_flush_duration_seconds",
			Help:      "Time taken to flush metadata batch to database and pubsub.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
		}, []string{"reason"}),

		BatchesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batches_total",
			Help:      "Total number of metadata batches flushed.",
		}, []string{"reason"}),

		DroppedKeysTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_dropped_keys_total",
			Help:      "Total number of metadata keys dropped due to capacity limits.",
		}),

		MetadataTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_flushed_total",
			Help:      "Total number of unique metadatas flushed.",
		}),

		PublishErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_publish_errors_total",
			Help:      "Total number of metadata batch pubsub publish calls that have resulted in an error.",
		}),
	}
}

func (m Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.BatchUtilization,
		m.BatchSize,
		m.FlushDuration,
		m.BatchesTotal,
		m.DroppedKeysTotal,
		m.MetadataTotal,
		m.PublishErrors,
	}
}

func (m Metrics) register(reg prometheus.Registerer) {
	if reg != nil {
		reg.MustRegister(m.BatchUtilization)
		reg.MustRegister(m.BatchSize)
		reg.MustRegister(m.FlushDuration)
		reg.MustRegister(m.DroppedKeysTotal)
		reg.MustRegister(m.BatchesTotal)
		reg.MustRegister(m.MetadataTotal)
		reg.MustRegister(m.PublishErrors)
	}
}
