package workspacetraffic

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	BytesReadTotal      prometheus.CounterVec
	BytesWrittenTotal   prometheus.CounterVec
	ReadErrorsTotal     prometheus.CounterVec
	WriteErrorsTotal    prometheus.CounterVec
	ReadLatencySeconds  prometheus.HistogramVec
	WriteLatencySeconds prometheus.HistogramVec
	LabelNames          []string
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	m := &Metrics{
		BytesReadTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_read_total",
		}, labelNames),
		BytesWrittenTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_written_total",
		}, labelNames),
		ReadErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "read_errors_total",
		}, labelNames),
		WriteErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "write_errors_total",
		}, labelNames),
		ReadLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "read_latency_seconds",
		}, labelNames),
		WriteLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "write_latency_seconds",
		}, labelNames),
	}

	reg.MustRegister(m.BytesReadTotal)
	reg.MustRegister(m.BytesWrittenTotal)
	reg.MustRegister(m.ReadErrorsTotal)
	reg.MustRegister(m.WriteErrorsTotal)
	reg.MustRegister(m.ReadLatencySeconds)
	reg.MustRegister(m.WriteLatencySeconds)
	return m
}

func (m *Metrics) ReadMetrics(lvs ...string) ConnMetrics {
	return &connMetrics{
		addError:       m.ReadErrorsTotal.WithLabelValues(lvs...).Add,
		observeLatency: m.ReadLatencySeconds.WithLabelValues(lvs...).Observe,
		addTotal:       m.BytesReadTotal.WithLabelValues(lvs...).Add,
	}
}

func (m *Metrics) WriteMetrics(lvs ...string) ConnMetrics {
	return &connMetrics{
		addError:       m.WriteErrorsTotal.WithLabelValues(lvs...).Add,
		observeLatency: m.WriteLatencySeconds.WithLabelValues(lvs...).Observe,
		addTotal:       m.BytesWrittenTotal.WithLabelValues(lvs...).Add,
	}
}

type ConnMetrics interface {
	AddError(float64)
	ObserveLatency(float64)
	AddTotal(float64)
	GetTotalBytes() int64
}

type connMetrics struct {
	addError       func(float64)
	observeLatency func(float64)
	addTotal       func(float64)
	total          int64
}

func (c *connMetrics) AddError(f float64) {
	c.addError(f)
}

func (c *connMetrics) ObserveLatency(f float64) {
	c.observeLatency(f)
}

func (c *connMetrics) AddTotal(f float64) {
	atomic.AddInt64(&c.total, int64(f))
	c.addTotal(f)
}

func (c *connMetrics) GetTotalBytes() int64 {
	return c.total
}
