package workspacetraffic

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	BytesRead      prometheus.CounterVec
	BytesWritten   prometheus.CounterVec
	Errors         prometheus.CounterVec
	ReadLatencyMS  prometheus.HistogramVec
	WriteLatencyMS prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		BytesRead: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_read",
		}, []string{"username", "workspace_name", "agent_name"}),
		BytesWritten: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "bytes_written",
		}, []string{"username", "workspace_name", "agent_name"}),
		Errors: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "errors",
		}, []string{"username", "workspace_name", "agent_name"}),
		ReadLatencyMS: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "read_latency_seconds",
		}, []string{"username", "workspace_name", "agent_name"}),
		WriteLatencyMS: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "write_latency_seconds",
		}, []string{"username", "workspace_name", "agent_name"}),
	}

	reg.MustRegister(m.BytesRead)
	reg.MustRegister(m.BytesWritten)
	reg.MustRegister(m.Errors)
	reg.MustRegister(m.ReadLatencyMS)
	reg.MustRegister(m.WriteLatencyMS)
	return m
}
