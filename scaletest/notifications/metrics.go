package notifications

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type NotificationType string

const (
	NotificationTypeWebsocket NotificationType = "websocket"
	NotificationTypeSMTP      NotificationType = "smtp"
)

type Metrics struct {
	notificationLatency *prometheus.HistogramVec
	notificationErrors  *prometheus.CounterVec
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
	}, []string{"notification_id", "notification_type"})
	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "notification_delivery_errors_total",
		Help:      "Total number of notification delivery errors",
	}, []string{"action"})

	reg.MustRegister(latency, errors)

	return &Metrics{
		notificationLatency: latency,
		notificationErrors:  errors,
	}
}

func (m *Metrics) RecordLatency(latency time.Duration, notificationID string, notificationType NotificationType) {
	m.notificationLatency.WithLabelValues(notificationID, string(notificationType)).Observe(latency.Seconds())
}

func (m *Metrics) AddError(action string) {
	m.notificationErrors.WithLabelValues(action).Inc()
}
