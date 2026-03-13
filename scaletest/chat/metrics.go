package chat

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	ChatCreateLatencySeconds        prometheus.HistogramVec
	ChatTimeToRunningSeconds        prometheus.HistogramVec
	ChatTimeToFirstOutputSeconds    prometheus.HistogramVec
	ChatTimeToTerminalStatusSeconds prometheus.HistogramVec
	ChatCreateErrorsTotal           prometheus.CounterVec
	ChatStreamErrorsTotal           prometheus.CounterVec
	ChatTerminalStatusTotal         prometheus.CounterVec
	ChatTurnsCompletedTotal         prometheus.CounterVec
	ChatRetryEventsTotal            prometheus.CounterVec
	ActiveChatStreams               prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	terminalStatusLabelNames := append(append([]string{}, labelNames...), "status")

	m := &Metrics{
		ChatCreateLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_create_latency_seconds",
			Help:      "Time in seconds to create a chat before streaming begins.",
		}, labelNames),
		ChatTimeToRunningSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_running_seconds",
			Help:      "Time in seconds from the start of a chat turn until the chat enters running status.",
		}, labelNames),
		ChatTimeToFirstOutputSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_first_output_seconds",
			Help:      "Time in seconds from chat creation start until the first output is received.",
		}, labelNames),
		ChatTimeToTerminalStatusSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_terminal_status_seconds",
			Help:      "Time in seconds from the start of a chat turn until a terminal status is received.",
		}, labelNames),
		ChatCreateErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_create_errors_total",
			Help:      "Total number of chat creation errors.",
		}, labelNames),
		ChatStreamErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_stream_errors_total",
			Help:      "Total number of chat stream errors.",
		}, labelNames),
		ChatTerminalStatusTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_terminal_status_total",
			Help:      "Total number of terminal chat statuses observed.",
		}, terminalStatusLabelNames),
		ChatTurnsCompletedTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_turns_completed_total",
			Help:      "Total number of chat turns (user→assistant exchanges) completed successfully.",
		}, labelNames),
		ChatRetryEventsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_retry_events_total",
			Help:      "Total number of chat retry events observed.",
		}, labelNames),
		ActiveChatStreams: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "active_chat_streams",
			Help:      "Current number of active chat streams.",
		}, labelNames),
	}

	reg.MustRegister(m.ChatCreateLatencySeconds)
	reg.MustRegister(m.ChatTimeToRunningSeconds)
	reg.MustRegister(m.ChatTimeToFirstOutputSeconds)
	reg.MustRegister(m.ChatTimeToTerminalStatusSeconds)
	reg.MustRegister(m.ChatCreateErrorsTotal)
	reg.MustRegister(m.ChatStreamErrorsTotal)
	reg.MustRegister(m.ChatTerminalStatusTotal)
	reg.MustRegister(m.ChatTurnsCompletedTotal)
	reg.MustRegister(m.ChatRetryEventsTotal)
	reg.MustRegister(m.ActiveChatStreams)

	return m
}
