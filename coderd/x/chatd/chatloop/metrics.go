package chatloop

import (
	"context"
	"errors"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

const (
	metricsNamespace = "coderd"
	metricsSubsystem = "chatd"

	// Label values for Chats.
	StateStreaming = "streaming"
	StateWaiting   = "waiting"

	// Label values for CompactionTotal.
	CompactionResultSuccess = "success"
	CompactionResultError   = "error"
	CompactionResultTimeout = "timeout"
)

// Metrics holds Prometheus metrics for the chatd subsystem.
type Metrics struct {
	Chats                    *prometheus.GaugeVec
	MessageCount             *prometheus.HistogramVec
	PromptSizeBytes          *prometheus.HistogramVec
	ToolResultSizeBytes      *prometheus.HistogramVec
	ToolErrorsTotal          *prometheus.CounterVec
	TTFTSeconds              *prometheus.HistogramVec
	CompactionTotal          *prometheus.CounterVec
	StepsTotal               *prometheus.CounterVec
	StreamRetriesTotal       *prometheus.CounterVec
	StreamBufferDroppedTotal prometheus.Counter
}

// NewMetrics creates a new Metrics instance registered with the
// given registerer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)
	return &Metrics{
		Chats: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "chats",
			Help:      "Number of chats being processed, by state.",
		}, []string{"state"}),
		MessageCount: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "message_count",
			Help:      "Number of messages in the prompt per LLM request.",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 11), // 1, 2, 4, ..., 1024
		}, []string{"provider", "model"}),
		PromptSizeBytes: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "prompt_size_bytes",
			Help:      "Estimated byte size of the prompt per LLM request.",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 10), // 1KB .. 256MB
		}, []string{"provider", "model"}),
		ToolResultSizeBytes: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "tool_result_size_bytes",
			Help:      "Size in bytes of each tool execution result.",
			Buckets:   prometheus.ExponentialBuckets(64, 4, 9), // 64B .. 4MB
		}, []string{"provider", "model", "tool_name"}),
		ToolErrorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "tool_errors_total",
			Help:      "Total tool calls that returned an error result.",
		}, []string{"provider", "model", "tool_name"}),
		TTFTSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "ttft_seconds",
			Help:      "Time-to-first-token: wall time from LLM request to first streamed chunk.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"provider", "model"}),
		CompactionTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "compaction_total",
			Help:      "Total compaction outcomes (only recorded when compaction was triggered or failed).",
		}, []string{"provider", "model", "result"}),
		StepsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "steps_total",
			Help:      "Total agentic loop steps across all chats.",
		}, []string{"provider", "model"}),
		StreamRetriesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stream_retries_total",
			Help:      "Total LLM stream retries.",
		}, []string{"provider", "model", "kind"}),
		StreamBufferDroppedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stream_buffer_dropped_total",
			Help:      "Number of chat stream buffer events dropped due to the per-chat buffer cap.",
		}),
	}
}

// NopMetrics returns a Metrics instance that discards all data.
// Useful for tests and when metrics collection is not desired.
func NopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}

// RecordCompaction classifies and records a compaction attempt.
// It is a no-op when m is nil.
func (m *Metrics) RecordCompaction(provider, model string, compacted bool, err error) {
	if m == nil {
		return
	}
	switch {
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		m.CompactionTotal.WithLabelValues(provider, model, CompactionResultTimeout).Inc()
	case err != nil && errors.Is(err, context.Canceled):
		// User interruption, not a compaction failure.
		return
	case err != nil:
		m.CompactionTotal.WithLabelValues(provider, model, CompactionResultError).Inc()
	case compacted:
		m.CompactionTotal.WithLabelValues(provider, model, CompactionResultSuccess).Inc()
		// !compacted && err == nil means threshold not reached -- not
		// recorded.
	}
}

// RecordStreamRetry increments stream_retries_total. The caller
// must obtain classified via chaterror.Classify (non-empty Kind).
// No-op when m is nil.
func (m *Metrics) RecordStreamRetry(provider, model string, classified chaterror.ClassifiedError) {
	if m == nil {
		return
	}
	m.StreamRetriesTotal.WithLabelValues(provider, model, classified.Kind).Inc()
}

// RecordToolError increments tool_errors_total for the given
// tool. No-op when m is nil.
func (m *Metrics) RecordToolError(provider, model, toolLabel string) {
	if m == nil {
		return
	}
	m.ToolErrorsTotal.WithLabelValues(provider, model, toolLabel).Inc()
}

// RecordStreamBufferDropped increments stream_buffer_dropped_total
// once per dropped event. No-op when m is nil.
func (m *Metrics) RecordStreamBufferDropped() {
	if m == nil {
		return
	}
	m.StreamBufferDroppedTotal.Inc()
}

// EstimatePromptSize returns a cheap byte-size estimate of a
// fantasy prompt by summing the text content lengths of all
// message parts. This avoids JSON marshaling overhead.
func EstimatePromptSize(messages []fantasy.Message) int {
	var size int
	for _, msg := range messages {
		for _, part := range msg.Content {
			size += ContentPartSize(part)
		}
	}
	return size
}

// ContentPartSize returns the byte length of a MessagePart's
// primary text or data field.
func ContentPartSize(part fantasy.MessagePart) int {
	switch p := part.(type) {
	case fantasy.TextPart:
		return len(p.Text)
	case fantasy.ReasoningPart:
		return len(p.Text)
	case fantasy.FilePart:
		return len(p.Data)
	case fantasy.ToolCallPart:
		return len(p.Input)
	case fantasy.ToolResultPart:
		return toolResultOutputSize(p.Output)
	default:
		return 0
	}
}

// ToolResultSize returns the byte length of a
// ToolResultContent's primary text or data field.
func ToolResultSize(r fantasy.ToolResultContent) int {
	return toolResultOutputSize(r.Result)
}

func toolResultOutputSize(output fantasy.ToolResultOutputContent) int {
	if output == nil {
		return 0
	}
	switch v := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return len(v.Text)
	case fantasy.ToolResultOutputContentError:
		if v.Error != nil {
			return len(v.Error.Error())
		}
		return 0
	case fantasy.ToolResultOutputContentMedia:
		return len(v.Data)
	default:
		return 0
	}
}
