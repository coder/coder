package chatloop

import (
	"context"
	"errors"
	"time"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

const (
	metricsNamespace = "coderd"
	metricsSubsystem = "chatd"

	// Label values for Chats.
	StateStreaming = "streaming"

	// Label values for CompactionTotal.
	CompactionResultSuccess = "success"
	CompactionResultError   = "error"
	CompactionResultTimeout = "timeout"

	// Label values for StageAnomaliesTotal.
	// StageAnomalyNegativeElapsed is a stage whose measured duration
	// was negative and was not observed.
	StageAnomalyNegativeElapsed = "negative_elapsed"
	// StageAnomalyInvertedWindow is a stage reconstructed from
	// timestamps whose end preceded its start, or which lacked one of
	// them, and was not observed.
	StageAnomalyInvertedWindow = "inverted_window"
	// StageAnomalyStaleAnchor is a turn whose trigger timestamp precedes
	// the anchor of the previous turn; the anchor was clamped.
	StageAnomalyStaleAnchor = "stale_anchor"
)

// basicStages is the set of stages observed into StageDurationSeconds
// at codersdk.ChatStageMetricsLevelBasic. It holds the wait, connect, and
// model-call stages; the stages that only describe chatd's own work
// inside a step (generation_step, prepare, thinking, compaction) are
// left to the full level.
var basicStages = map[Stage]struct{}{
	StageChatTurn:         {},
	StageQueueWait:        {},
	StageCapacityWait:     {},
	StageAcquisition:      {},
	StageMCPConnect:       {},
	StageStream:           {},
	StageTimeToFirstToken: {},
	StageProviderAttempt:  {},
	StageToolCall:         {},
	StageCommit:           {},
	StageRetryBackoff:     {},
}

// modelStages is the set of stages observed into
// ModelStageDurationSeconds at every level: the stages whose duration
// is the provider's work on a model.
var modelStages = map[Stage]struct{}{
	StageTimeToFirstToken: {},
	StageStream:           {},
	StageProviderAttempt:  {},
}

// fullModelStages holds the model-bound stages observed into
// ModelStageDurationSeconds only above codersdk.ChatStageMetricsLevelBasic.
var fullModelStages = map[Stage]struct{}{
	StageThinking:   {},
	StageCompaction: {},
}

// MetricsOptions configures which optional metric families NewMetrics
// registers.
type MetricsOptions struct {
	// StageMetrics selects the chat lifecycle stage families to expose.
	// Unrecognized or empty values mean codersdk.ChatStageMetricsLevelBasic.
	StageMetrics codersdk.ChatStageMetricsLevel
}

// Metrics holds Prometheus metrics for the chatd subsystem.
type Metrics struct {
	Chats                     *prometheus.GaugeVec
	MessageCount              *prometheus.HistogramVec
	PromptSizeBytes           *prometheus.HistogramVec
	ToolResultSizeBytes       *prometheus.HistogramVec
	ToolResultTruncatedTotal  *prometheus.CounterVec
	ToolErrorsTotal           *prometheus.CounterVec
	TTFTSeconds               *prometheus.HistogramVec
	StageMetricsLevel         *prometheus.GaugeVec
	StageDurationSeconds      *prometheus.HistogramVec
	ModelStageDurationSeconds *prometheus.HistogramVec
	StageAnomaliesTotal       *prometheus.CounterVec
	CompactionTotal           *prometheus.CounterVec
	StepsTotal                *prometheus.CounterVec
	StreamRetriesTotal        *prometheus.CounterVec
	FindToolsCallsTotal       prometheus.Counter
	FindToolsEmptyTotal       prometheus.Counter
	FindToolsMatchCount       prometheus.Histogram
	FindToolsActivationsTotal prometheus.Counter

	// stageMetrics is the level the stage families were built for.
	stageMetrics codersdk.ChatStageMetricsLevel
}

// NewMetrics creates a new Metrics instance registered with the
// given registerer, with every stage metric family enabled.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	return NewMetricsWithOptions(reg, MetricsOptions{StageMetrics: codersdk.ChatStageMetricsLevelFull})
}

// NewMetricsWithOptions creates a new Metrics instance registered with
// the given registerer. Stage families that opts leaves disabled are
// still constructed, against no registerer, so every recorder can be
// called at any level; they simply never appear in a scrape.
func NewMetricsWithOptions(reg prometheus.Registerer, opts MetricsOptions) *Metrics {
	level := codersdk.NewChatStageMetricsLevelFromString(string(opts.StageMetrics))
	factory := promauto.With(reg)
	// stageFactory registers the families exposed at basic and full.
	stageFactory := factory
	if level == codersdk.ChatStageMetricsLevelOff {
		stageFactory = promauto.With(nil)
	}
	m := &Metrics{
		stageMetrics: level,
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
		ToolResultTruncatedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "tool_result_truncated_total",
			Help:      "Total tool results truncated to fit the model context window.",
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
		StageMetricsLevel: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stage_metrics_level",
			Help:      "Configured chat stage metrics level: 1 for the configured level and 0 for the others (off, basic, or full). Tells dashboards and alerts which stage and turn families this replica exposes.",
		}, []string{"level"}),
		StageDurationSeconds: stageFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stage_duration_seconds",
			Help:      "Wall time spent in each chat lifecycle stage. Stages overlap in wall time; this is a stage-time profile, not a partition of the turn. The scope label separates stages that run inside a chat turn from detached background work. The chat_kind label is empty for stages recorded without a known chat. At the basic stage metrics level only the turn, wait, connect, model-call, tool, and commit stages are observed.",
			Buckets:   stageDurationBuckets(level),
		}, []string{"stage", "scope", "chat_kind"}),
		ModelStageDurationSeconds: stageFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "model_stage_duration_seconds",
			Help:      "Wall time spent in the chat lifecycle stages that are a provider's work on a model: time_to_first_token, stream, and provider_attempt, plus thinking and compaction at the full stage metrics level. Every observation here is also observed on stage_duration_seconds. provider_type is the configured type of the model's AI provider (for example bedrock), not the wire protocol the provider label of other chatd metrics reports. chat_kind is empty for stages recorded without a known chat.",
			Buckets:   stageDurationBuckets(level),
		}, []string{"stage", "provider_type", "chat_kind", "model"}),
		StageAnomaliesTotal: stageFactory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stage_anomalies_total",
			Help:      "Chat lifecycle stage observations dropped or adjusted by reason. Reasons: negative_elapsed and inverted_window (clock inconsistencies), stale_anchor (turn anchor clamped to the previous turn's anchor).",
		}, []string{"reason"}),
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
		FindToolsCallsTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "find_tools_calls_total",
			Help:      "Total find_tools calls.",
		}),
		FindToolsEmptyTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "find_tools_empty_total",
			Help:      "Total find_tools calls with no matches.",
		}),
		FindToolsMatchCount: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "find_tools_match_count",
			Help:      "Number of matches returned by find_tools calls.",
			Buckets:   prometheus.LinearBuckets(0, 2, 11),
		}),
		FindToolsActivationsTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "find_tools_activations_total",
			Help:      "Total deferred tool activations returned by find_tools.",
		}),
	}
	// Every level value gets a series so a replica restarted at a new
	// level overwrites the old one on the next scrape.
	for _, value := range codersdk.ChatStageMetricsLevelValues {
		var set float64
		if value == string(level) {
			set = 1
		}
		m.StageMetricsLevel.WithLabelValues(value).Set(set)
	}
	return m
}

// stageDurationBuckets returns the stage histogram buckets for level.
// The basic ladder is dense between 100ms and 10s; the full ladder
// adds edges at 50ms and between 20s and 10min. Full is a superset of
// basic, so a query reads the same edges at either level.
func stageDurationBuckets(level codersdk.ChatStageMetricsLevel) []float64 {
	if level == codersdk.ChatStageMetricsLevelFull {
		return []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300, 600, 1800, 3600}
	}
	return []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800, 3600}
}

// NopMetrics returns a Metrics instance that discards all data.
// Useful for tests and when metrics collection is not desired.
func NopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}

// RecordStageDuration observes one chat lifecycle stage duration.
// chatKind is empty when the stage was recorded without a known chat.
// Negative durations are dropped and counted as an anomaly at every
// level. At the basic level, stages outside basicStages are dropped
// silently. Stages in the model set for the level whose model is known
// are additionally observed on ModelStageDurationSeconds. No-op when m
// is nil.
func (m *Metrics) RecordStageDuration(stage Stage, scope Scope, chatKind ChatKind, model StageModel, elapsed time.Duration) {
	if m == nil {
		return
	}
	if elapsed < 0 {
		m.RecordStageAnomaly(StageAnomalyNegativeElapsed)
		return
	}
	basic := m.stageMetrics == codersdk.ChatStageMetricsLevelBasic
	if basic {
		if _, ok := basicStages[stage]; !ok {
			return
		}
	}
	seconds := elapsed.Seconds()
	m.StageDurationSeconds.WithLabelValues(string(stage), string(scope), string(chatKind)).Observe(seconds)
	if model.Model == "" {
		return
	}
	_, modelStage := modelStages[stage]
	if !modelStage && !basic {
		_, modelStage = fullModelStages[stage]
	}
	if modelStage {
		m.ModelStageDurationSeconds.WithLabelValues(string(stage), model.ProviderType, string(chatKind), model.Model).Observe(seconds)
	}
}

// RecordStageAnomaly counts a stage observation that was dropped, by
// reason. No-op when m is nil.
func (m *Metrics) RecordStageAnomaly(reason string) {
	if m == nil {
		return
	}
	m.StageAnomaliesTotal.WithLabelValues(reason).Inc()
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
	m.StreamRetriesTotal.WithLabelValues(
		provider,
		model,
		string(classified.Kind),
	).Inc()
}

// RecordToolError increments tool_errors_total for the given
// tool. No-op when m is nil.
func (m *Metrics) RecordToolError(provider, model, toolLabel string) {
	if m == nil {
		return
	}
	m.ToolErrorsTotal.WithLabelValues(provider, model, toolLabel).Inc()
}

// RecordToolResultTruncated increments tool_result_truncated_total for
// the given tool. No-op when m is nil. An empty tool label is
// normalized to "unknown" to match the other tool metrics.
func (m *Metrics) RecordToolResultTruncated(provider, model, toolLabel string) {
	if m == nil {
		return
	}
	if toolLabel == "" {
		toolLabel = "unknown"
	}
	m.ToolResultTruncatedTotal.WithLabelValues(provider, model, toolLabel).Inc()
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
