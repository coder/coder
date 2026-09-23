package chatloop

import (
	"context"
	"errors"
	"time"

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
	// StageAnomalyNonPositiveTurn is a finished turn whose duration
	// was not positive, so its accounting was not emitted.
	StageAnomalyNonPositiveTurn = "nonpositive_turn"
	// StageAnomalyOverattributed is a finished turn whose categories
	// summed to more than its duration. The categories were emitted as
	// measured, with no unattributed remainder.
	StageAnomalyOverattributed = "overattributed"
)

// observedStages is the set of stages observed into
// StageDurationSeconds without FullStageMetrics. It holds the wait,
// connect, model-call, tool, and commit stages; the stages that only
// describe chatd's own work inside a step (generation_step, prepare,
// thinking, compaction) are span-only.
var observedStages = map[Stage]struct{}{
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
// ModelStageDurationSeconds: the stages whose duration is the
// provider's work on a model.
var modelStages = map[Stage]struct{}{
	StageTimeToFirstToken: {},
	StageStream:           {},
	StageProviderAttempt:  {},
}

// fullModelStages holds the model-bound stages observed into
// ModelStageDurationSeconds only with FullStageMetrics.
var fullModelStages = map[Stage]struct{}{
	StageThinking:   {},
	StageCompaction: {},
}

// stageDurationBuckets are the edges of both stage histograms, dense
// between 100ms and 10s and sparse out to an hour.
var stageDurationBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800, 3600}

// fullStageDurationBuckets is a superset of stageDurationBuckets that
// adds edges at 50ms and between 20s and 10min, so a query written
// against either ladder reads the same edges.
var fullStageDurationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300, 600, 1800, 3600}

// turnDurationBuckets are the edges for histograms of per-turn sums:
// 1s to 1h. Sub-second resolution carries no information for a value
// summed over a whole turn.
var turnDurationBuckets = []float64{1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1800, 3600}

// MetricsOptions configures which optional metric families NewMetrics
// registers.
type MetricsOptions struct {
	// StageMetrics registers the chat lifecycle stage families. When
	// false they are still constructed, against no registerer, so every
	// recorder can be called; they never appear in a scrape.
	StageMetrics bool
	// FullStageMetrics widens the stage families: every stage is
	// observed on the stage histograms, both use the 16-edge ladder, and
	// the per-turn distribution families are registered. It has no
	// effect when StageMetrics is false.
	FullStageMetrics bool
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
	StageDurationSeconds      *prometheus.HistogramVec
	ModelStageDurationSeconds *prometheus.HistogramVec
	TurnTimeSecondsTotal      *prometheus.CounterVec
	TurnsTotal                *prometheus.CounterVec
	TurnOutcomesTotal         *prometheus.CounterVec
	TurnStageSeconds          *prometheus.HistogramVec
	TurnTimeSeconds           *prometheus.HistogramVec
	TurnTimeShare             *prometheus.HistogramVec
	StageAnomaliesTotal       *prometheus.CounterVec
	CompactionTotal           *prometheus.CounterVec
	StepsTotal                *prometheus.CounterVec
	StreamRetriesTotal        *prometheus.CounterVec
	FindToolsCallsTotal       prometheus.Counter
	FindToolsEmptyTotal       prometheus.Counter
	FindToolsMatchCount       prometheus.Histogram
	FindToolsActivationsTotal prometheus.Counter

	// full is whether every stage is observed and the per-turn
	// distribution families are registered.
	full bool
}

// NewMetrics creates a new Metrics instance registered with the
// given registerer, with the stage metric families enabled and the
// full set disabled.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	return NewMetricsWithOptions(reg, MetricsOptions{StageMetrics: true})
}

// NewMetricsWithOptions creates a new Metrics instance registered with
// the given registerer.
func NewMetricsWithOptions(reg prometheus.Registerer, opts MetricsOptions) *Metrics {
	factory := promauto.With(reg)
	stageFactory := factory
	if !opts.StageMetrics {
		stageFactory = promauto.With(nil)
	}
	full := opts.StageMetrics && opts.FullStageMetrics
	// fullFactory registers the per-turn distribution families.
	fullFactory := stageFactory
	if !full {
		fullFactory = promauto.With(nil)
	}
	buckets := stageDurationBuckets
	if full {
		buckets = fullStageDurationBuckets
	}
	m := &Metrics{
		full: full,
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
		StageDurationSeconds: stageFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stage_duration_seconds",
			Help:      "Wall time spent in each chat lifecycle stage. Stages overlap in wall time; this is a stage-time profile, not a partition of the turn. The scope label separates stages that run inside a chat turn from detached background work. The chat_kind label is empty for stages recorded without a known chat. Without the chat-stage-metrics-full experiment only the turn, wait, connect, model-call, tool, and commit stages are observed; the others are span-only.",
			Buckets:   buckets,
		}, []string{"stage", "scope", "chat_kind"}),
		ModelStageDurationSeconds: stageFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "model_stage_duration_seconds",
			Help:      "Wall time spent in the chat lifecycle stages that are a provider's work on a model: time_to_first_token, stream, and provider_attempt, plus thinking and compaction with the chat-stage-metrics-full experiment. Every observation here is also observed on stage_duration_seconds. provider_type is the configured type of the model's AI provider (for example bedrock), not the wire protocol the provider label of other chatd metrics reports. chat_kind is empty for stages recorded without a known chat.",
			Buckets:   buckets,
		}, []string{"stage", "provider_type", "chat_kind", "model"}),
		TurnTimeSecondsTotal: stageFactory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turn_time_seconds_total",
			Help:      "Accumulated wall time of finished chat turns split into disjoint categories that sum to the turn duration, added once per turn per category when the turn ends. Divide by turns_total for mean seconds per turn. Only turns that finished normally are counted.",
		}, []string{"category", "chat_kind"}),
		TurnsTotal: stageFactory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turns_total",
			Help:      "Total chat turns whose time partition was recorded in turn_time_seconds_total. Only turns that finished normally are counted.",
		}, []string{"chat_kind"}),
		TurnOutcomesTotal: stageFactory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turn_outcomes_total",
			Help:      "Total chat turns by outcome (completed, interrupted, error, abandoned); every closed turn is counted exactly once, and completed turns are the ones whose time partition is recorded.",
		}, []string{"outcome", "chat_kind"}),
		TurnStageSeconds: fullFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turn_stage_seconds",
			Help:      "Total wall time one chat turn spent in a stage, observed once per turn when the turn ends. Stages overlap, so these do not partition the turn. Only turns that finished normally are counted. Registered only with the chat-stage-metrics-full experiment.",
			Buckets:   turnDurationBuckets,
		}, []string{"stage", "chat_kind"}),
		TurnTimeSeconds: fullFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turn_time_seconds",
			Help:      "Wall time of one chat turn split into disjoint categories that sum to the turn duration, observed once per turn per category when the turn ends. Every category is observed, including the ones with no time. Only turns that finished normally are counted. Registered only with the chat-stage-metrics-full experiment.",
			Buckets:   turnDurationBuckets,
		}, []string{"category", "chat_kind"}),
		TurnTimeShare: fullFactory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "turn_time_share",
			Help:      "Fraction of a chat turn's wall time in each disjoint category, observed once per turn per category when the turn ends. The shares of one turn sum to 1. Only turns that finished normally are counted. Registered only with the chat-stage-metrics-full experiment.",
			Buckets:   prometheus.LinearBuckets(0, 0.05, 21),
		}, []string{"category", "chat_kind"}),
		StageAnomaliesTotal: stageFactory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "stage_anomalies_total",
			Help:      "Chat lifecycle stage observations dropped, adjusted, or emitted with a known inconsistency, by reason. Reasons: negative_elapsed and inverted_window (clock inconsistencies), stale_anchor (turn anchor clamped to the previous turn's anchor), nonpositive_turn (finished turn whose accounting was not emitted), overattributed (turn whose categories summed to more than its duration).",
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
	return m
}

// NopMetrics returns a Metrics instance that discards all data.
// Useful for tests and when metrics collection is not desired.
func NopMetrics() *Metrics {
	return NewMetrics(prometheus.NewRegistry())
}

// RecordStageDuration observes one chat lifecycle stage duration.
// chatKind is empty when the stage was recorded without a known chat.
// Negative durations are dropped and counted as an anomaly. Without
// full metrics, stages outside observedStages are dropped silently.
// Stages in the model set whose model is known are additionally
// observed on ModelStageDurationSeconds. No-op when m is nil.
func (m *Metrics) RecordStageDuration(stage Stage, scope Scope, chatKind ChatKind, model StageModel, elapsed time.Duration) {
	if m == nil {
		return
	}
	if elapsed < 0 {
		m.RecordStageAnomaly(StageAnomalyNegativeElapsed)
		return
	}
	if !m.full {
		if _, ok := observedStages[stage]; !ok {
			return
		}
	}
	seconds := elapsed.Seconds()
	m.StageDurationSeconds.WithLabelValues(string(stage), string(scope), string(chatKind)).Observe(seconds)
	if model.Model == "" {
		return
	}
	_, modelStage := modelStages[stage]
	if !modelStage && m.full {
		_, modelStage = fullModelStages[stage]
	}
	if modelStage {
		m.ModelStageDurationSeconds.WithLabelValues(string(stage), model.ProviderType, string(chatKind), model.Model).Observe(seconds)
	}
}

// RecordStageAnomaly counts a stage observation that was dropped or
// emitted inconsistent, by reason. No-op when m is nil.
func (m *Metrics) RecordStageAnomaly(reason string) {
	if m == nil {
		return
	}
	m.StageAnomaliesTotal.WithLabelValues(reason).Inc()
}

// RecordTurnStage observes the total time one turn spent in a stage.
// No-op when m is nil.
func (m *Metrics) RecordTurnStage(stage Stage, chatKind ChatKind, elapsed time.Duration) {
	if m == nil || elapsed < 0 {
		return
	}
	m.TurnStageSeconds.WithLabelValues(string(stage), string(chatKind)).Observe(elapsed.Seconds())
}

// RecordTurnCategory records one category of a turn's time partition:
// it adds the elapsed time to the category counter and observes the
// elapsed time and its fraction of the turn on the per-turn histograms.
// Categories with no time are recorded as zero so every category has a
// series and the shares of a turn sum to 1. No-op when m is nil.
func (m *Metrics) RecordTurnCategory(category Category, chatKind ChatKind, elapsed time.Duration, share float64) {
	if m == nil || elapsed < 0 {
		return
	}
	m.TurnTimeSecondsTotal.WithLabelValues(string(category), string(chatKind)).Add(elapsed.Seconds())
	m.TurnTimeSeconds.WithLabelValues(string(category), string(chatKind)).Observe(elapsed.Seconds())
	m.TurnTimeShare.WithLabelValues(string(category), string(chatKind)).Observe(share)
}

// RecordTurn counts one finished turn whose time partition was
// recorded through RecordTurnCategory. No-op when m is nil.
func (m *Metrics) RecordTurn(chatKind ChatKind) {
	if m == nil {
		return
	}
	m.TurnsTotal.WithLabelValues(string(chatKind)).Inc()
}

// RecordTurnOutcome counts one closed turn by outcome. No-op when m is
// nil.
func (m *Metrics) RecordTurnOutcome(outcome TurnOutcome, chatKind ChatKind) {
	if m == nil {
		return
	}
	m.TurnOutcomesTotal.WithLabelValues(string(outcome), string(chatKind)).Inc()
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
