package chatloop

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/coder/quartz"
)

// Stage names a chat lifecycle stage and its span.
// Only observedStages are also histogram `stage` labels.
type Stage string

// Stage values.
const (
	StageChatTurn         Stage = "chat_turn"
	StageQueueWait        Stage = "queue_wait"
	StageAcquisition      Stage = "acquisition"
	StageGenerationStep   Stage = "generation_step"
	StagePrepare          Stage = "prepare"
	StageMCPConnect       Stage = "mcp_connect"
	StageProviderAttempt  Stage = "provider_attempt"
	StageStream           Stage = "stream"
	StageTimeToFirstToken Stage = "time_to_first_token"
	StageThinking         Stage = "thinking"
	StageToolCall         Stage = "tool_call"
	StageCommit           Stage = "commit"
	StageCompaction       Stage = "compaction"
	StageRetryBackoff     Stage = "retry_backoff"
)

// GenerationActionExecuteLocalTools is the generation_action value of
// a step that runs local tools. A step with this action attributes its
// own time to tool execution.
const GenerationActionExecuteLocalTools = "execute_local_tools"

// Span attribute keys. Keys are lowercase snake_case and shared by
// every stage that carries the value.
const (
	AttrProvider            = "provider"
	AttrProviderType        = "provider_type"
	AttrModel               = "model"
	AttrReasoningEffort     = "reasoning_effort"
	AttrChatID              = "chat_id"
	AttrChatKind            = "chat_kind"
	AttrGenerationAttempt   = "generation_attempt"
	AttrGenerationAction    = "generation_action"
	AttrToolName            = "tool_name"
	AttrHTTPStatusCode      = "http_status_code"
	AttrHTTPMethod          = "http_method"
	AttrCompactionSource    = "compaction_source"
	AttrScope               = "scope"
	AttrTurnOutcome         = "turn_outcome"
	AttrMCPServersConnected = "mcp_servers_connected"
	AttrMCPServersFailed    = "mcp_servers_failed"
)

// TurnOutcome is how a chat turn closed.
type TurnOutcome string

// TurnOutcome values.
const (
	TurnOutcomeCompleted   TurnOutcome = "completed"
	TurnOutcomeInterrupted TurnOutcome = "interrupted"
	TurnOutcomeError       TurnOutcome = "error"
	// TurnOutcomeAbandoned covers any other close, such as a newer prompt
	// or runner exit.
	TurnOutcomeAbandoned TurnOutcome = "abandoned"
)

// Scope separates stages attributable to a prompt (turn) from detached
// work (background).
type Scope string

// Scope values.
const (
	ScopeTurn       Scope = "turn"
	ScopeBackground Scope = "background"
)

// ChatKind is the chat type of a stage; empty when the chat is unknown.
type ChatKind string

// ChatKind values.
const (
	ChatKindRoot     ChatKind = "root"
	ChatKindSubagent ChatKind = "subagent"
)

const tracerName = "chatd"

// StageTracer emits a span per stage and, for observed stages, a
// histogram sample over the same window. A nil *StageTracer discards
// everything.
type StageTracer struct {
	tracer  trace.Tracer
	metrics *Metrics
	clock   quartz.Clock
}

// StageTracerOption configures a StageTracer.
type StageTracerOption func(*StageTracer)

// WithClock sets the tracer's clock.
func WithClock(clock quartz.Clock) StageTracerOption {
	return func(t *StageTracer) {
		t.clock = clock
	}
}

// NewStageTracer builds a stage tracer; nil arguments discard their output.
func NewStageTracer(provider trace.TracerProvider, metrics *Metrics, opts ...StageTracerOption) *StageTracer {
	if provider == nil {
		provider = noop.NewTracerProvider()
	}
	t := &StageTracer{
		tracer:  provider.Tracer(tracerName),
		metrics: metrics,
		clock:   quartz.NewReal(),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *StageTracer) otelTracer() trace.Tracer {
	if t == nil || t.tracer == nil {
		return noop.NewTracerProvider().Tracer(tracerName)
	}
	return t.tracer
}

// Now returns the current time from the tracer's clock.
func (t *StageTracer) Now() time.Time {
	if t == nil || t.clock == nil {
		return time.Now()
	}
	return t.clock.Now()
}

// StageModel identifies the model a stage ran against.
type StageModel struct {
	// Provider is the wire protocol; ProviderType is the configured
	// provider type (e.g. "bedrock"), which can differ.
	Provider     string
	ProviderType string
	Model        string
	Effort       string
}

func (m StageModel) attributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 4)
	if m.Provider != "" {
		attrs = append(attrs, attribute.String(AttrProvider, m.Provider))
	}
	if m.ProviderType != "" {
		attrs = append(attrs, attribute.String(AttrProviderType, m.ProviderType))
	}
	if m.Model != "" {
		attrs = append(attrs, attribute.String(AttrModel, m.Model))
	}
	if m.Effort != "" {
		attrs = append(attrs, attribute.String(AttrReasoningEffort, m.Effort))
	}
	return attrs
}

// StageSpan is an in-flight stage. Only the first End call takes
// effect. Not safe for concurrent use.
type StageSpan struct {
	tracer   *StageTracer
	stage    Stage
	scope    Scope
	chatKind ChatKind
	model    StageModel
	span     trace.Span
	start    time.Time
	ended    bool
	// acc is the turn the stage runs in, nil outside a turn.
	acc *TurnAccumulator
	// node is the stage's place in the turn's attribution tree, nil
	// for stages that do not partition turn time.
	node *stageNode
}

type stageScopeKey struct{}

type stageChatKindKey struct{}

// ContextWithScope returns ctx carrying scope for stages started on it.
// It is a context value, not a span property, so it works with a no-op
// tracer provider.
func ContextWithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, stageScopeKey{}, scope)
}

// ContextWithChatKind returns ctx carrying kind for stages started on it.
func ContextWithChatKind(ctx context.Context, kind ChatKind) context.Context {
	return context.WithValue(ctx, stageChatKindKey{}, kind)
}

// scopeFromContext defaults to background when ctx has no scope.
func scopeFromContext(ctx context.Context) Scope {
	if scope, ok := ctx.Value(stageScopeKey{}).(Scope); ok && scope != "" {
		return scope
	}
	return ScopeBackground
}

func chatKindFromContext(ctx context.Context) ChatKind {
	kind, _ := ctx.Value(stageChatKindKey{}).(ChatKind)
	return kind
}

// Start begins a stage span as a child of the span in ctx.
func (t *StageTracer) Start(
	ctx context.Context,
	stage Stage,
	attrs ...attribute.KeyValue,
) (context.Context, *StageSpan) {
	return t.startSpan(ctx, stage, scopeFromContext(ctx), time.Time{},
		[]trace.SpanStartOption{trace.WithAttributes(attrs...)})
}

// StartRootAt begins a turn-scoped span in a new trace, backdated to
// start so earlier recorded stages fall inside it. A zero or future
// start means now; a future start is counted as an anomaly.
func (t *StageTracer) StartRootAt(
	ctx context.Context,
	stage Stage,
	start time.Time,
	attrs ...attribute.KeyValue,
) (context.Context, *StageSpan) {
	return t.startSpan(ctx, stage, ScopeTurn, start, []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithAttributes(attrs...),
	})
}

func (t *StageTracer) startSpan(
	ctx context.Context,
	stage Stage,
	scope Scope,
	start time.Time,
	opts []trace.SpanStartOption,
) (context.Context, *StageSpan) {
	if t == nil {
		return ctx, nil
	}
	now := t.Now()
	switch {
	case start.IsZero():
		start = now
	case start.After(now):
		// Clock skew from another replica.
		t.recordAnomalyIfObserved(stage, StageAnomalyFutureStart)
		start = now
	}
	opts = append(opts, trace.WithTimestamp(start))
	chatKind := chatKindFromContext(ctx)
	opts = append(opts, trace.WithAttributes(stageIdentityAttributes(scope, chatKind)...))
	// Only turn-scoped stages report to the turn on ctx. A background
	// stage may run on a context derived from a turn's, and its time is
	// not the turn's.
	var acc *TurnAccumulator
	if scope == ScopeTurn {
		acc = turnAccumulatorFromContext(ctx)
	}
	var node *stageNode
	if acc != nil {
		if _, attributing := attributingStages[stage]; attributing {
			node = &stageNode{stage: stage, parent: stageNodeFromContext(ctx)}
			ctx = context.WithValue(ctx, stageNodeKey{}, node)
		}
	}
	ctx, span := t.otelTracer().Start(ContextWithScope(ctx, scope), string(stage), opts...)
	return ctx, &StageSpan{
		tracer:   t,
		stage:    stage,
		scope:    scope,
		chatKind: chatKind,
		span:     span,
		start:    start,
		acc:      acc,
		node:     node,
	}
}

func stageIdentityAttributes(scope Scope, chatKind ChatKind) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String(AttrScope, string(scope))}
	if chatKind != "" {
		attrs = append(attrs, attribute.String(AttrChatKind, string(chatKind)))
	}
	return attrs
}

// SetAttributes adds attributes to the stage span.
func (s *StageSpan) SetAttributes(attrs ...attribute.KeyValue) {
	if s == nil || s.ended {
		return
	}
	s.span.SetAttributes(attrs...)
}

// SetModel sets the model for the span and its histogram sample.
func (s *StageSpan) SetModel(model StageModel) {
	if s == nil || s.ended {
		return
	}
	s.model = model
	s.acc.setModel(model)
	s.span.SetAttributes(model.attributes()...)
}

// SetGenerationAction records the action a generation step took, on
// the span and on the step's turn attribution, where it decides
// whether the step's own time counts as tool execution.
func (s *StageSpan) SetGenerationAction(action string) {
	if s == nil || s.ended {
		return
	}
	s.node.setAction(action)
	s.span.SetAttributes(attribute.String(AttrGenerationAction, action))
}

// SpanContext returns the span context of the stage span, which is
// invalid when tracing is not configured.
func (s *StageSpan) SpanContext() trace.SpanContext {
	if s == nil {
		return trace.SpanContext{}
	}
	return s.span.SpanContext()
}

// End closes the span, records its duration, and returns the elapsed
// window. Calls after the first return zero.
func (s *StageSpan) End(err error) time.Duration {
	s.adoptTurnModel()
	elapsed, ok := s.closeSpan(err)
	if !ok {
		return 0
	}
	s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
	s.report(elapsed, err)
	return elapsed
}

// EndWithoutObservation closes the span without recording a duration.
func (s *StageSpan) EndWithoutObservation(err error) {
	if elapsed, ok := s.closeSpan(err); ok {
		s.report(elapsed, err)
	}
}

// adoptTurnModel stamps the first model the turn resolved on the
// chat_turn span when the root itself never received one.
func (s *StageSpan) adoptTurnModel() {
	if s == nil || s.stage != StageChatTurn || s.model.Model != "" {
		return
	}
	if model := s.acc.Model(); model.Model != "" {
		s.SetModel(model)
	}
}

// EndTurn closes a chat_turn span at end. Only completed turns are
// observed on the histogram.
func (s *StageSpan) EndTurn(outcome TurnOutcome, err error, end time.Time) {
	if s == nil || s.ended {
		return
	}
	s.span.SetAttributes(attribute.String(AttrTurnOutcome, string(outcome)))
	s.adoptTurnModel()
	elapsed := s.closeSpanAt(err, end)
	if outcome == TurnOutcomeCompleted {
		s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
	}
	s.report(elapsed, err)
}

func (s *StageSpan) closeSpan(err error) (elapsed time.Duration, ok bool) {
	if s == nil || s.ended {
		return 0, false
	}
	return s.closeSpanAt(err, s.tracer.Now()), true
}

func (s *StageSpan) closeSpanAt(err error, end time.Time) time.Duration {
	s.ended = true
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End(trace.WithTimestamp(end))
	return end.Sub(s.start)
}

// Record emits a finished stage span with explicit timestamps. Windows
// with a zero timestamp or end before start are dropped as anomalies.
func (t *StageTracer) Record(
	ctx context.Context,
	stage Stage,
	model StageModel,
	start, end time.Time,
	err error,
	attrs ...attribute.KeyValue,
) {
	if start.IsZero() || end.IsZero() {
		t.recordAnomalyIfObserved(stage, StageAnomalyMissingTimestamp)
		return
	}
	if end.Before(start) {
		t.recordAnomalyIfObserved(stage, StageAnomalyInvertedWindow)
		return
	}
	scope := scopeFromContext(ctx)
	chatKind := chatKindFromContext(ctx)
	_, span := t.otelTracer().Start(ctx, string(stage),
		trace.WithTimestamp(start),
		trace.WithAttributes(attrs...),
		trace.WithAttributes(model.attributes()...),
		trace.WithAttributes(stageIdentityAttributes(scope, chatKind)...),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End(trace.WithTimestamp(end))
	t.observe(stage, scope, chatKind, model, end.Sub(start))
	if scope == ScopeTurn {
		recordAttribution(ctx, stage, end.Sub(start))
	}
}

// RecordAnomaly counts a dropped or adjusted stage observation.
func (t *StageTracer) RecordAnomaly(reason StageAnomaly) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.recordStageAnomaly(reason)
}

func (t *StageTracer) recordAnomalyIfObserved(stage Stage, reason StageAnomaly) {
	if _, ok := observedStages[stage]; !ok {
		return
	}
	t.RecordAnomaly(reason)
}

func (t *StageTracer) recordTurnOutcome(outcome TurnOutcome, chatKind ChatKind) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordTurnOutcome(outcome, chatKind)
}

func (t *StageTracer) observe(stage Stage, scope Scope, chatKind ChatKind, model StageModel, elapsed time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.recordStageDuration(stage, scope, chatKind, model, elapsed)
}
