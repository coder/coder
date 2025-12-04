package bridge

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	bridgeErrors *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "bridge_errors_total",
		Help:      "Total number of bridge errors",
	}, []string{"action"})

	reg.MustRegister(errors)

	return &Metrics{
		bridgeErrors: errors,
	}
}

func (m *Metrics) AddError(action string) {
	m.bridgeErrors.WithLabelValues(action).Inc()
}
