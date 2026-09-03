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

// Stage names. Every value is both a span name and the `stage` label
// value recorded on stage_duration_seconds.
const (
	StageChatTurn         = "chat_turn"
	StageQueueWait        = "queue_wait"
	StageCapacityWait     = "capacity_wait"
	StageAcquisition      = "acquisition"
	StageGenerationStep   = "generation_step"
	StagePrepare          = "prepare"
	StageMCPConnect       = "mcp_connect"
	StageProviderAttempt  = "provider_attempt"
	StageStream           = "stream"
	StageTimeToFirstToken = "time_to_first_token"
	StageThinking         = "thinking"
	StageToolCall         = "tool_call"
	StageCommit           = "commit"
	StageCompaction       = "compaction"
	StageRetryBackoff     = "retry_backoff"
)

// GenerationActionExecuteLocalTools is the generation_action value of
// a step that runs local tools. Turn accounting compares the value
// passed to SetGenerationAction against it to separate tool execution
// from chatd overhead.
const GenerationActionExecuteLocalTools = "execute_local_tools"

// Span attribute keys. Keys are lowercase snake_case and shared by
// every stage that carries the value.
const (
	AttrProvider          = "provider"
	AttrModel             = "model"
	AttrReasoningEffort   = "reasoning_effort"
	AttrChatID            = "chat_id"
	AttrChatKind          = "chat_kind"
	AttrGenerationAttempt = "generation_attempt"
	AttrGenerationAction  = "generation_action"
	AttrToolName          = "tool_name"
	AttrHTTPStatusCode    = "http_status_code"
	AttrHTTPMethod        = "http_method"
	AttrHTTPHost          = "http_host"
	AttrCompactionSource  = "compaction_source"
	AttrScope             = "scope"
)

// Scope values. A stage is turn scoped when it runs inside a chat
// turn's trace, and background scoped when it runs on work detached
// from the turn.
const (
	ScopeTurn       = "turn"
	ScopeBackground = "background"
)

// Chat kind attribute values.
const (
	ChatKindRoot     = "root"
	ChatKindSubagent = "subagent"
)

// tracerName is the instrumentation scope reported on chatd spans.
const tracerName = "github.com/coder/coder/v2/coderd/x/chatd"

// StageTracer emits one span and one stage_duration_seconds
// observation per chat lifecycle stage. Both are produced from the
// same call so span and histogram durations cannot diverge.
//
// A nil *StageTracer is usable and discards everything.
type StageTracer struct {
	tracer  trace.Tracer
	metrics *Metrics
	// clock is the time source for the stage windows measured here.
	// Stages recorded from explicit timestamps do not use it.
	clock quartz.Clock
}

// NewStageTracer builds a stage tracer from a tracer provider and the
// chatd metrics. A nil provider falls back to a no-op tracer and nil
// metrics to a discarding registry, so callers without tracing or
// metrics configured still get a usable tracer.
func NewStageTracer(provider trace.TracerProvider, metrics *Metrics) *StageTracer {
	if provider == nil {
		provider = noop.NewTracerProvider()
	}
	if metrics == nil {
		metrics = NopMetrics()
	}
	return &StageTracer{
		tracer:  provider.Tracer(tracerName),
		metrics: metrics,
		clock:   quartz.NewReal(),
	}
}

// NopStageTracer returns a stage tracer that discards spans and
// metrics.
func NopStageTracer() *StageTracer {
	return NewStageTracer(nil, nil)
}

func (t *StageTracer) otelTracer() trace.Tracer {
	if t == nil || t.tracer == nil {
		return noop.NewTracerProvider().Tracer(tracerName)
	}
	return t.tracer
}

// Now returns the current time from the tracer's clock, the time
// source for the stage windows it measures.
func (t *StageTracer) Now() time.Time {
	if t == nil || t.clock == nil {
		return time.Now()
	}
	return t.clock.Now()
}

// StageModel identifies the model a stage ran against. Both fields
// are empty for stages that run before a model is resolved, such as
// the queue and capacity waits. Effort is the effective reasoning
// effort sent to the provider, empty when the model config sets none.
type StageModel struct {
	Model  string
	Effort string
}

// attributes returns the span attributes for the identity, omitting
// the ones that are unknown.
func (m StageModel) attributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	if m.Model != "" {
		attrs = append(attrs, attribute.String(AttrModel, m.Model))
	}
	if m.Effort != "" {
		attrs = append(attrs, attribute.String(AttrReasoningEffort, m.Effort))
	}
	return attrs
}

// StageSpan is an in-flight stage. End must be called exactly once;
// the duration observation happens there.
type StageSpan struct {
	tracer   *StageTracer
	stage    string
	scope    string
	chatKind string
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

// stageScopeKey keys the stage scope carried by a context. It is
// private so the scope can only be set through ContextWithScope.
type stageScopeKey struct{}

// stageChatKindKey keys the chat kind carried by a context. It is
// private so the chat kind can only be set through
// ContextWithChatKind.
type stageChatKindKey struct{}

// ContextWithScope returns ctx carrying scope for the stages started
// on it. The scope is a plain context value rather than a property of
// the span in ctx, so it survives configurations where spans are not
// recorded, such as a no-op tracer provider.
func ContextWithScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, stageScopeKey{}, scope)
}

// ContextWithChatKind returns ctx carrying kind for the stages started
// on it. Callers that hold the chat row set it once, on the context a
// turn runs on or on the context of a single stage recorded outside a
// turn, and every stage derived from that context carries the value.
func ContextWithChatKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, stageChatKindKey{}, kind)
}

// scopeFromContext reads the scope ContextWithScope put on ctx.
// Contexts with no scope are background scoped, so work detached from
// a turn is kept out of the turn profile.
func scopeFromContext(ctx context.Context) string {
	if scope, ok := ctx.Value(stageScopeKey{}).(string); ok && scope != "" {
		return scope
	}
	return ScopeBackground
}

// chatKindFromContext reads the chat kind ContextWithChatKind put on
// ctx. It is empty when the stage runs without a known chat, which
// keeps the label present but unset rather than guessing a kind.
func chatKindFromContext(ctx context.Context) string {
	kind, _ := ctx.Value(stageChatKindKey{}).(string)
	return kind
}

// Start begins a stage span as a child of the span in ctx and returns
// a context carrying it. The stage takes the scope and chat kind on
// ctx, so stages started on a context with no scope are background
// scoped.
func (t *StageTracer) Start(
	ctx context.Context,
	stage string,
	attrs ...attribute.KeyValue,
) (context.Context, *StageSpan) {
	return t.startSpan(ctx, stage, scopeFromContext(ctx), time.Time{},
		[]trace.SpanStartOption{trace.WithAttributes(attrs...)})
}

// StartRoot begins a stage span in its own trace, ignoring any span
// in ctx. links records the relationship to the originating span
// context instead of making that span the parent, so the stage's
// trace stays scoped to the chat turn. The span opens a turn, so it
// is turn scoped regardless of what ctx carries.
func (t *StageTracer) StartRoot(
	ctx context.Context,
	stage string,
	links []trace.Link,
	attrs ...attribute.KeyValue,
) (context.Context, *StageSpan) {
	return t.StartRootAt(ctx, stage, time.Time{}, links, attrs...)
}

// StartRootAt begins a root stage span that started at an earlier,
// already known instant. The span timestamp and the recorded duration
// both run from start, so stages reconstructed inside the span still
// fall within it. A zero start means the span begins now.
func (t *StageTracer) StartRootAt(
	ctx context.Context,
	stage string,
	start time.Time,
	links []trace.Link,
	attrs ...attribute.KeyValue,
) (context.Context, *StageSpan) {
	return t.startSpan(ctx, stage, ScopeTurn, start, []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithLinks(links...),
		trace.WithAttributes(attrs...),
	})
}

func (t *StageTracer) startSpan(
	ctx context.Context,
	stage string,
	scope string,
	start time.Time,
	opts []trace.SpanStartOption,
) (context.Context, *StageSpan) {
	if t == nil {
		return ctx, nil
	}
	now := t.Now()
	if start.IsZero() || start.After(now) {
		start = now
	} else {
		opts = append(opts, trace.WithTimestamp(start))
	}
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
	ctx, span := t.otelTracer().Start(ContextWithScope(ctx, scope), stage, opts...)
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

// stageIdentityAttributes returns the attributes every stage span
// carries. An unknown chat kind is omitted from the span, where an
// absent attribute reads better than an empty one.
func stageIdentityAttributes(scope, chatKind string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String(AttrScope, scope)}
	if chatKind != "" {
		attrs = append(attrs, attribute.String(AttrChatKind, chatKind))
	}
	return attrs
}

// SetAttributes adds attributes to the stage span. It is a no-op
// after End.
func (s *StageSpan) SetAttributes(attrs ...attribute.KeyValue) {
	if s == nil || s.ended {
		return
	}
	s.span.SetAttributes(attrs...)
}

// SetModel records the model identity on the span and on the
// duration observation End makes, for stages that learn the model
// after they start.
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

// End closes the stage span, records its duration, and marks the span
// as errored when err is non-nil. Calls after the first are ignored so
// a deferred End cannot double-count a stage.
func (s *StageSpan) End(err error) {
	s.adoptTurnModel()
	if elapsed, ok := s.closeSpan(err); ok {
		s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
		s.addTurnStageTotal(elapsed)
		s.report(elapsed, err)
	}
}

// EndWithoutObservation closes the stage span exactly as End does but
// makes no duration observation. It is for stages whose window is only
// comparable across runs when it completed, so a truncated window
// would skew the histogram while the span still needs to report the
// failure.
func (s *StageSpan) EndWithoutObservation(err error) {
	if elapsed, ok := s.closeSpan(err); ok {
		s.report(elapsed, err)
	}
}

// adoptTurnModel gives the turn's root stage the model identity that
// the turn resolved after the root started, so the root is labeled
// like the stages inside it.
func (s *StageSpan) adoptTurnModel() {
	if s == nil || s.stage != StageChatTurn || s.model.Model != "" {
		return
	}
	if model := s.acc.Model(); model.Model != "" {
		s.SetModel(model)
	}
}

// closeSpan ends the span and returns the window it covered. ok is
// false for a nil span and for calls after the first, so a deferred
// end cannot double-count a stage.
func (s *StageSpan) closeSpan(err error) (elapsed time.Duration, ok bool) {
	if s == nil || s.ended {
		return 0, false
	}
	s.ended = true
	elapsed = s.tracer.Now().Sub(s.start)
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
	return elapsed, true
}

// Record emits an already-finished stage span with explicit start and
// end timestamps. It is for stages whose boundaries are only known
// after the fact, such as durations reconstructed from persisted
// timestamps. The stage takes the scope and chat kind on ctx.
// Non-positive or unset windows are dropped.
func (t *StageTracer) Record(
	ctx context.Context,
	stage string,
	model StageModel,
	start, end time.Time,
	err error,
	attrs ...attribute.KeyValue,
) {
	t.RecordAs(ctx, stage, scopeFromContext(ctx), model, start, end, err, attrs...)
}

// RecordAs is Record with an explicit scope, for stages recorded on a
// context that does not carry the scope they belong to.
func (t *StageTracer) RecordAs(
	ctx context.Context,
	stage string,
	scope string,
	model StageModel,
	start, end time.Time,
	err error,
	attrs ...attribute.KeyValue,
) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		t.recordAnomaly(StageAnomalyInvertedWindow)
		return
	}
	chatKind := chatKindFromContext(ctx)
	_, span := t.otelTracer().Start(ctx, stage,
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

func (t *StageTracer) recordAnomaly(reason string) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageAnomaly(reason)
}

func (t *StageTracer) observe(stage, scope, chatKind string, model StageModel, elapsed time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageDuration(stage, scope, chatKind, model.Model, model.Effort, elapsed)
}
