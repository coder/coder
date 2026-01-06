package agentapi

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type MetadataBatcherMetrics struct {
	batchUtilization prometheus.Histogram
	droppedKeysTotal prometheus.Counter
	batchesTotal     *prometheus.CounterVec
	batchSize        prometheus.Histogram
	flushDuration    *prometheus.HistogramVec
}

func NewMetadataBatcherMetrics() *MetadataBatcherMetrics {
	// Native histogram configuration (matching provisionerdserver pattern).
	nativeHistogramOpts := func(opts prometheus.HistogramOpts) prometheus.HistogramOpts {
		opts.NativeHistogramBucketFactor = 1.1
		opts.NativeHistogramMaxBucketNumber = 100
		opts.NativeHistogramMinResetDuration = time.Hour
		opts.NativeHistogramZeroThreshold = 0
		opts.NativeHistogramMaxZeroThreshold = 0
		return opts
	}

	return &MetadataBatcherMetrics{
		batchUtilization: prometheus.NewHistogram(nativeHistogramOpts(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_utilization",
			Help:      "Number of metadata keys per agent in each flushed batch.",
		})),

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
		}, []string{"reason", "success"}),

		batchSize: prometheus.NewHistogram(nativeHistogramOpts(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_batch_size",
			Help:      "Total number of metadata entries in each flushed batch.",
		})),

		flushDuration: prometheus.NewHistogramVec(nativeHistogramOpts(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "agentapi",
			Name:      "metadata_flush_duration_seconds",
			Help:      "Time taken to flush metadata batch to database and pubsub.",
		}), []string{"reason"}),
	}
}

func (m *MetadataBatcherMetrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.batchUtilization,
		m.droppedKeysTotal,
		m.batchesTotal,
		m.batchSize,
		m.flushDuration,
	}
}
