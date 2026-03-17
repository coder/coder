package chat

import "github.com/prometheus/client_golang/prometheus"

const (
	MetricLabelRunID = "run_id"

	metricLabelPhase  = "phase"
	metricLabelStatus = "status"
	metricLabelStage  = "stage"

	phaseInitial  = "initial"
	phaseFollowUp = "follow_up"

	failureStageCreateChat    = "create_chat"
	failureStageCreateMessage = "create_message"
	failureStageStreamOpen    = "stream_open"
	failureStageStreamEvent   = "stream_event"
	failureStageStreamClosed  = "stream_closed"
	failureStageStatusError   = "status_error"
)

var (
	chatRequestLatencyBuckets    = prometheus.ExponentialBucketsRange(0.05, 120, 18)
	chatProcessingLatencyBuckets = prometheus.ExponentialBucketsRange(0.1, 300, 18)
)

func MetricLabelNames() []string {
	return []string{MetricLabelRunID}
}

func MetricLabelValues(runID string) []string {
	return []string{runID}
}

type Metrics struct {
	ChatCreateLatencySeconds        prometheus.HistogramVec
	ChatMessageLatencySeconds       prometheus.HistogramVec
	ChatConversationDurationSeconds prometheus.HistogramVec
	ChatTimeToRunningSeconds        prometheus.HistogramVec
	ChatTimeToFirstOutputSeconds    prometheus.HistogramVec
	ChatTimeToTerminalStatusSeconds prometheus.HistogramVec
	ChatCreateErrorsTotal           prometheus.CounterVec
	ChatStreamErrorsTotal           prometheus.CounterVec
	ChatStageFailuresTotal          prometheus.CounterVec
	ChatTerminalStatusTotal         prometheus.CounterVec
	ChatTurnsCompletedTotal         prometheus.CounterVec
	ChatRetryEventsTotal            prometheus.CounterVec
	ActiveChatStreams               prometheus.GaugeVec
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	phaseLabelNames := append(append([]string{}, labelNames...), metricLabelPhase)
	terminalStatusLabelNames := append(append([]string{}, labelNames...), metricLabelStatus)
	failureStageLabelNames := append(append([]string{}, labelNames...), metricLabelStage)

	m := &Metrics{
		ChatCreateLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_create_latency_seconds",
			Help:      "Time in seconds to create a chat and enqueue the initial turn.",
			Buckets:   chatRequestLatencyBuckets,
		}, labelNames),
		ChatMessageLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_message_latency_seconds",
			Help:      "Time in seconds to add a follow-up message to an existing chat.",
			Buckets:   chatRequestLatencyBuckets,
		}, phaseLabelNames),
		ChatConversationDurationSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_conversation_duration_seconds",
			Help:      "Time in seconds from chat creation start until the conversation finishes or errors.",
			Buckets:   chatProcessingLatencyBuckets,
		}, labelNames),
		ChatTimeToRunningSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_running_seconds",
			Help:      "Time in seconds from the start of a chat turn until the chat enters running status.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
		ChatTimeToFirstOutputSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_first_output_seconds",
			Help:      "Time in seconds from the start of a chat turn until the first output is received.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
		ChatTimeToTerminalStatusSeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_terminal_status_seconds",
			Help:      "Time in seconds from the start of a chat turn until a terminal status is received.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
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
			Help:      "Total number of chat stream errors or disconnections.",
		}, labelNames),
		ChatStageFailuresTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_stage_failures_total",
			Help:      "Total number of stage-specific chat runner failures.",
		}, failureStageLabelNames),
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
	reg.MustRegister(m.ChatMessageLatencySeconds)
	reg.MustRegister(m.ChatConversationDurationSeconds)
	reg.MustRegister(m.ChatTimeToRunningSeconds)
	reg.MustRegister(m.ChatTimeToFirstOutputSeconds)
	reg.MustRegister(m.ChatTimeToTerminalStatusSeconds)
	reg.MustRegister(m.ChatCreateErrorsTotal)
	reg.MustRegister(m.ChatStreamErrorsTotal)
	reg.MustRegister(m.ChatStageFailuresTotal)
	reg.MustRegister(m.ChatTerminalStatusTotal)
	reg.MustRegister(m.ChatTurnsCompletedTotal)
	reg.MustRegister(m.ChatRetryEventsTotal)
	reg.MustRegister(m.ActiveChatStreams)

	return m
}
