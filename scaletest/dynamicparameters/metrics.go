package dynamicparameters

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	LatencyInitialResponseSeconds prometheus.HistogramVec
	LatencyChangeResponseSeconds  prometheus.HistogramVec
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	m := &Metrics{
		LatencyInitialResponseSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "dynamic_parameters_latency_initial_response_seconds",
			Help:      "Time in seconds to get the initial dynamic parameters response from start of request.",
		}, labelNames),
		LatencyChangeResponseSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "dynamic_parameters_latency_change_response_seconds",
			Help:      "Time in seconds to between sending a dynamic parameters change request and receiving the response.",
		}, labelNames),
	}
	reg.MustRegister(m.LatencyInitialResponseSeconds)
	reg.MustRegister(m.LatencyChangeResponseSeconds)
	return m
}
