package dashboard

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	DurationSeconds *prometheus.HistogramVec
	Errors          *prometheus.CounterVec
	Statuses        *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		DurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest_dashboard",
			Name:      "duration_seconds",
		}, []string{"action"}),
		Errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest_dashboard",
			Name:      "errors_total",
		}, []string{"action"}),
		Statuses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest_dashboard",
			Name:      "statuses_total",
		}, []string{"action", "code"}),
	}

	reg.MustRegister(m.DurationSeconds)
	reg.MustRegister(m.Errors)
	reg.MustRegister(m.Statuses)
	return m
}
