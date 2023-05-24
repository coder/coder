package workspacetraffic

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	BytesRead           prometheus.CounterVec
	BytesWritten        prometheus.CounterVec
	Errors              prometheus.CounterVec
	ReadLatencySeconds  prometheus.HistogramVec
	WriteLatencySeconds prometheus.HistogramVec
	LabelNames          []string
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	m := &Metrics{
		BytesRead: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_read",
		}, labelNames),
		BytesWritten: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_written",
		}, labelNames),
		Errors: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "errors",
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

	reg.MustRegister(m.BytesRead)
	reg.MustRegister(m.BytesWritten)
	reg.MustRegister(m.Errors)
	reg.MustRegister(m.ReadLatencySeconds)
	reg.MustRegister(m.WriteLatencySeconds)
	return m
}
