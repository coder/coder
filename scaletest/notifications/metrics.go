package notifications

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	notificationLatency *prometheus.HistogramVec
	notificationErrors  *prometheus.CounterVec
	missedNotifications *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "notification_delivery_latency_seconds",
		Help:      "Time between notification-creating action and receipt of notification by client",
	}, []string{"username", "notification_type"})
	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "notification_delivery_errors_total",
		Help:      "Total number of notification delivery errors",
	}, []string{"username", "action"})
	missed := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "notification_delivery_missed_total",
		Help:      "Total number of missed notifications",
	}, []string{"username"})

	reg.MustRegister(latency, errors, missed)

	return &Metrics{
		notificationLatency: latency,
		notificationErrors:  errors,
		missedNotifications: missed,
	}
}

func (m *Metrics) RecordLatency(latency time.Duration, username, notificationType string) {
	m.notificationLatency.WithLabelValues(username, notificationType).Observe(latency.Seconds())
}

func (m *Metrics) AddError(username, action string) {
	m.notificationErrors.WithLabelValues(username, action).Inc()
}

func (m *Metrics) RecordMissed(username string) {
	m.missedNotifications.WithLabelValues(username).Inc()
}
