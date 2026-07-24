package chat

import "github.com/prometheus/client_golang/prometheus"

const (
	metricLabelPhase  = "phase"
	metricLabelStatus = "status"
	metricLabelStage  = "stage"

	phaseInitial  = "initial"
	phaseFollowUp = "follow_up"

	failureStageCreateChat       = "create_chat"
	failureStageCreateMessage    = "create_message"
	failureStageStreamOpen       = "stream_open"
	failureStageStreamEndedEarly = "stream_ended_early"
	failureStageStatusError      = "status_error"
)

var (
	chatRequestLatencyBuckets    = prometheus.ExponentialBucketsRange(0.05, 120, 18)
	chatProcessingLatencyBuckets = prometheus.ExponentialBucketsRange(0.1, 300, 18)
)

// Metrics holds the Prometheus metrics emitted by the chat scaletest.
type Metrics struct {
	ChatCreateLatencySeconds        prometheus.Histogram
	ChatMessageLatencySeconds       *prometheus.HistogramVec
	ChatConversationDurationSeconds prometheus.Histogram
	ChatTimeToRunningSeconds        *prometheus.HistogramVec
	ChatTimeToFirstOutputSeconds    *prometheus.HistogramVec
	ChatTimeToTerminalStatusSeconds *prometheus.HistogramVec
	ChatStageFailuresTotal          *prometheus.CounterVec
	ChatTerminalStatusTotal         *prometheus.CounterVec
	ChatTurnsCompletedTotal         prometheus.Counter
	ChatRetryEventsTotal            prometheus.Counter
	ActiveChatStreams               prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	phaseLabelNames := []string{metricLabelPhase}
	terminalStatusLabelNames := []string{metricLabelStatus}
	failureStageLabelNames := []string{metricLabelStage}

	m := &Metrics{
		ChatCreateLatencySeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_create_latency_seconds",
			Help:      "Time in seconds to create a chat and enqueue the initial turn.",
			Buckets:   chatRequestLatencyBuckets,
		}),
		ChatMessageLatencySeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_message_latency_seconds",
			Help:      "Time in seconds to add a follow-up message to an existing chat.",
			Buckets:   chatRequestLatencyBuckets,
		}, phaseLabelNames),
		ChatConversationDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_conversation_duration_seconds",
			Help:      "Time in seconds from chat creation start until the conversation finishes or errors.",
			Buckets:   chatProcessingLatencyBuckets,
		}),
		ChatTimeToRunningSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_running_seconds",
			Help:      "Time in seconds from the start of a chat turn until the chat enters running status.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
		ChatTimeToFirstOutputSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_first_output_seconds",
			Help:      "Time in seconds from the start of a chat turn until the first output is received.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
		ChatTimeToTerminalStatusSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_time_to_terminal_status_seconds",
			Help:      "Time in seconds from the start of a chat turn until a terminal status is received.",
			Buckets:   chatProcessingLatencyBuckets,
		}, phaseLabelNames),
		ChatStageFailuresTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_stage_failures_total",
			Help:      "Total number of terminal stage-specific chat runner failures.",
		}, failureStageLabelNames),
		ChatTerminalStatusTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_terminal_status_total",
			Help:      "Total number of terminal chat statuses observed.",
		}, terminalStatusLabelNames),
		ChatTurnsCompletedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_turns_completed_total",
			Help:      "Total number of chat turns completed successfully.",
		}),
		ChatRetryEventsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "chat_retry_events_total",
			Help:      "Total number of chat retry events observed.",
		}),
		ActiveChatStreams: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "active_chat_streams",
			Help:      "Current number of active chat streams.",
		}),
	}

	reg.MustRegister(m.ChatCreateLatencySeconds)
	reg.MustRegister(m.ChatMessageLatencySeconds)
	reg.MustRegister(m.ChatConversationDurationSeconds)
	reg.MustRegister(m.ChatTimeToRunningSeconds)
	reg.MustRegister(m.ChatTimeToFirstOutputSeconds)
	reg.MustRegister(m.ChatTimeToTerminalStatusSeconds)
	reg.MustRegister(m.ChatStageFailuresTotal)
	reg.MustRegister(m.ChatTerminalStatusTotal)
	reg.MustRegister(m.ChatTurnsCompletedTotal)
	reg.MustRegister(m.ChatRetryEventsTotal)
	reg.MustRegister(m.ActiveChatStreams)

	return m
}
