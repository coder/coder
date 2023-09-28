package dashboard

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	ObserveDuration(action string, d time.Duration)
	IncErrors(action string)
}

type PromMetrics struct {
	durationSeconds *prometheus.HistogramVec
	errors          *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *PromMetrics {
	m := &PromMetrics{
		durationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest_dashboard",
			Name:      "duration_seconds",
		}, []string{"action"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest_dashboard",
			Name:      "errors_total",
		}, []string{"action"}),
	}

	reg.MustRegister(m.durationSeconds)
	reg.MustRegister(m.errors)
	return m
}

func (p *PromMetrics) ObserveDuration(action string, d time.Duration) {
	p.durationSeconds.WithLabelValues(action).Observe(d.Seconds())
}

func (p *PromMetrics) IncErrors(action string) {
	p.errors.WithLabelValues(action).Inc()
}
