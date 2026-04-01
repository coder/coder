package chatd

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	relayOpenSourceInitial            = "initial"
	relayOpenSourceStatusNotification = "status_notification"
	relayOpenSourceReconnect          = "reconnect"

	relayOpenResultSuccess            = "success"
	relayOpenResultDialError          = "dial_error"
	relayOpenResultCanceled           = "canceled"
	relayOpenResultStaleResultDropped = "stale_result_dropped"

	relayCloseReasonStatusNotRemoteRunning = "status_not_remote_running"
	relayCloseReasonSuperseded             = "superseded"
	relayCloseReasonRelayPartsClosed       = "relay_parts_closed"
	relayCloseReasonContextDone            = "context_done"

	relayReconnectReasonDialFailed       = "dial_failed"
	relayReconnectReasonRelayPartsClosed = "relay_parts_closed"
	relayReconnectReasonDBGetChatFailed  = "db_get_chat_failed"

	relayReconnectPollResultDBError            = "db_error"
	relayReconnectPollResultStillRemoteRunning = "still_remote_running"
	relayReconnectPollResultNotRemoteRunning   = "not_remote_running"
)

type relayMetrics struct {
	relayOpen               *prometheus.CounterVec
	relayClose              *prometheus.CounterVec
	relayReconnectScheduled *prometheus.CounterVec
	relayReconnectPoll      *prometheus.CounterVec
	relayActive             prometheus.Gauge
}

func newRelayMetrics(reg prometheus.Registerer) *relayMetrics {
	m := &relayMetrics{
		relayOpen: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "relay_open_total",
			Help:      "Total number of chat stream relay open attempts, partitioned by source and result.",
		}, []string{"source", "result"}),
		relayClose: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "relay_close_total",
			Help:      "Total number of chat stream relay closes, partitioned by reason.",
		}, []string{"reason"}),
		relayReconnectScheduled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "relay_reconnect_scheduled_total",
			Help:      "Total number of chat stream relay reconnect attempts scheduled, partitioned by reason.",
		}, []string{"reason"}),
		relayReconnectPoll: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "relay_reconnect_poll_total",
			Help:      "Total number of chat stream relay reconnect polls, partitioned by result.",
		}, []string{"result"}),
		relayActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "chat_stream",
			Name:      "relay_active",
			Help:      "Current number of active chat stream relay connections.",
		}),
	}

	m.relayOpen = registerRelayCounterVec(reg, m.relayOpen)
	m.relayClose = registerRelayCounterVec(reg, m.relayClose)
	m.relayReconnectScheduled = registerRelayCounterVec(reg, m.relayReconnectScheduled)
	m.relayReconnectPoll = registerRelayCounterVec(reg, m.relayReconnectPoll)
	m.relayActive = registerRelayGauge(reg, m.relayActive)
	return m
}

func registerRelayCounterVec(reg prometheus.Registerer, collector *prometheus.CounterVec) *prometheus.CounterVec {
	if reg == nil {
		return collector
	}
	if err := reg.Register(collector); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec)
			if ok {
				return existing
			}
		}
	}
	return collector
}

func registerRelayGauge(reg prometheus.Registerer, collector prometheus.Gauge) prometheus.Gauge {
	if reg == nil {
		return collector
	}
	if err := reg.Register(collector); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			existing, ok := alreadyRegistered.ExistingCollector.(prometheus.Gauge)
			if ok {
				return existing
			}
		}
	}
	return collector
}

func (m *relayMetrics) observeRelayOpen(source, result string) {
	if m == nil || source == "" || result == "" {
		return
	}
	m.relayOpen.WithLabelValues(source, result).Inc()
}

func (m *relayMetrics) observeRelayClose(reason string) {
	if m == nil || reason == "" {
		return
	}
	m.relayClose.WithLabelValues(reason).Inc()
}

func (m *relayMetrics) observeRelayReconnectScheduled(reason string) {
	if m == nil || reason == "" {
		return
	}
	m.relayReconnectScheduled.WithLabelValues(reason).Inc()
}

func (m *relayMetrics) observeRelayReconnectPoll(result string) {
	if m == nil || result == "" {
		return
	}
	m.relayReconnectPoll.WithLabelValues(result).Inc()
}

func (m *relayMetrics) observeRelayActivated() {
	if m == nil {
		return
	}
	m.relayActive.Inc()
}

func (m *relayMetrics) observeRelayDeactivated() {
	if m == nil {
		return
	}
	m.relayActive.Dec()
}
