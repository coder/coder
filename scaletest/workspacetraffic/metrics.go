package workspacetraffic

import "github.com/prometheus/client_golang/prometheus"

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
