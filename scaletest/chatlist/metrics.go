package chatlist

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds the Prometheus metrics emitted by the chat list scaletest.
type Metrics struct {
	ListLatencySeconds prometheus.Histogram
	ListErrorsTotal    prometheus.Counter
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		ListLatencySeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_list_latency_seconds",
			Help:      "Time in seconds to list a page of chats.",
			Buckets:   prometheus.ExponentialBucketsRange(0.005, 30, 18),
		}),
		ListErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_list_errors_total",
			Help:      "Total number of failed chat list requests.",
		}),
	}

	reg.MustRegister(m.ListLatencySeconds)
	reg.MustRegister(m.ListErrorsTotal)
	return m
}
