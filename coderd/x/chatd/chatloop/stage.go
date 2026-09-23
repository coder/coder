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

// Stage names a chat lifecycle stage. Every value is both a span name
// and the `stage` label value recorded on stage_duration_seconds.
type Stage string

// Stage values.
const (
	StageChatTurn         Stage = "chat_turn"
	StageQueueWait        Stage = "queue_wait"
	StageCapacityWait     Stage = "capacity_wait"
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

// Span attribute keys. Keys are lowercase snake_case and shared by
// every stage that carries the value.
const (
	AttrProvider          = "provider"
	AttrProviderType      = "provider_type"
	AttrModel             = "model"
	AttrReasoningEffort   = "reasoning_effort"
	AttrChatID            = "chat_id"
	AttrChatKind          = "chat_kind"
	AttrOrganizationName  = "organization_name"
	AttrGenerationAttempt = "generation_attempt"
	AttrGenerationAction  = "generation_action"
	AttrToolName          = "tool_name"
	AttrHTTPStatusCode    = "http_status_code"
	AttrHTTPMethod        = "http_method"
	AttrHTTPHost          = "http_host"
	AttrCompactionSource  = "compaction_source"
	AttrScope             = "scope"
)

// Scope is the `scope` label and span attribute of a stage. A stage is
// turn scoped when it runs inside a chat turn's trace, and background
// scoped when it runs on work detached from the turn.
type Scope string

// Scope values.
const (
	ScopeTurn       Scope = "turn"
	ScopeBackground Scope = "background"
)

// ChatKind is the `chat_kind` label and span attribute of a stage. It
// is empty for stages recorded without a known chat.
type ChatKind string

// ChatKind values.
const (
	ChatKindRoot     ChatKind = "root"
	ChatKindSubagent ChatKind = "subagent"
)

// tracerName is the instrumentation scope reported on chatd spans.
const tracerName = "chatd"

// StageTracer emits one span and one stage_duration_seconds
// observation per chat lifecycle stage. Both are produced from the
// same call so span and histogram durations cannot diverge.
//
// A nil *StageTracer is usable and discards everything.
type StageTracer struct {
	tracer  trace.Tracer
	metrics *Metrics
	clock   quartz.Clock
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

// StageModel identifies the model a stage ran against. All fields are
// empty for stages that run before a model is resolved, such as the
// queue and capacity waits. ProviderType is the configured type of the
// AI provider the model config points at, such as "bedrock"; it is not
// the wire protocol the client speaks. Effort is the effective
// reasoning effort sent to the provider, empty when the model config
// sets none.
type StageModel struct {
	ProviderType string
	Model        string
	Effort       string
}

// attributes returns the span attributes for the identity, omitting
// the ones that are unknown.
func (m StageModel) attributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
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

// StageSpan is an in-flight stage. End must be called exactly once;
// the duration observation happens there.
type StageSpan struct {
	tracer   *StageTracer
	stage    Stage
	scope    Scope
	chatKind ChatKind
	model    StageModel
	span     trace.Span
	start    time.Time
	ended    bool
}

// stageScopeKey keys the stage scope carried by a context.
type stageScopeKey struct{}

// stageChatKindKey keys the chat kind carried by a context.
type stageChatKindKey struct{}

// stageOrganizationKey keys the organization name carried by a
// context.
type stageOrganizationKey struct{}

// ContextWithScope returns ctx carrying scope for the stages started
// on it. The scope is a plain context value rather than a property of
// the span in ctx, so it survives configurations where spans are not
// recorded, such as a no-op tracer provider.
func ContextWithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, stageScopeKey{}, scope)
}

// ContextWithChatKind returns ctx carrying kind for the stages started
// on it and on contexts derived from it.
func ContextWithChatKind(ctx context.Context, kind ChatKind) context.Context {
	return context.WithValue(ctx, stageChatKindKey{}, kind)
}

// ContextWithOrganization returns ctx carrying the name of the
// organization that owns the chat for the stages started on it and on
// contexts derived from it.
func ContextWithOrganization(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, stageOrganizationKey{}, name)
}

// scopeFromContext reads the scope ContextWithScope put on ctx.
// Contexts with no scope are background scoped, so work detached from
// a turn is kept out of the turn profile.
func scopeFromContext(ctx context.Context) Scope {
	if scope, ok := ctx.Value(stageScopeKey{}).(Scope); ok && scope != "" {
		return scope
	}
	return ScopeBackground
}

// chatKindFromContext reads the chat kind ContextWithChatKind put on
// ctx. It is empty when the stage runs without a known chat, which
// keeps the label present but unset rather than guessing a kind.
func chatKindFromContext(ctx context.Context) ChatKind {
	kind, _ := ctx.Value(stageChatKindKey{}).(ChatKind)
	return kind
}

// organizationFromContext reads the organization name
// ContextWithOrganization put on ctx. It is empty when the stage runs
// without a known chat.
func organizationFromContext(ctx context.Context) string {
	name, _ := ctx.Value(stageOrganizationKey{}).(string)
	return name
}

// Start begins a stage span as a child of the span in ctx and returns
// a context carrying it. The stage takes the scope and chat kind on
// ctx, so stages started on a context with no scope are background
// scoped.
func (t *StageTracer) Start(
	ctx context.Context,
	stage Stage,
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
	stage Stage,
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
	stage Stage,
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
	stage Stage,
	scope Scope,
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
	opts = append(opts, trace.WithAttributes(stageIdentityAttributes(scope, chatKind, organizationFromContext(ctx))...))
	ctx, span := t.otelTracer().Start(ContextWithScope(ctx, scope), string(stage), opts...)
	return ctx, &StageSpan{
		tracer:   t,
		stage:    stage,
		scope:    scope,
		chatKind: chatKind,
		span:     span,
		start:    start,
	}
}

// stageIdentityAttributes returns the attributes every stage span
// carries. An unknown chat kind or organization is omitted from the
// span, where an absent attribute reads better than an empty one.
func stageIdentityAttributes(scope Scope, chatKind ChatKind, organization string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String(AttrScope, string(scope))}
	if chatKind != "" {
		attrs = append(attrs, attribute.String(AttrChatKind, string(chatKind)))
	}
	if organization != "" {
		attrs = append(attrs, attribute.String(AttrOrganizationName, organization))
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
	s.span.SetAttributes(model.attributes()...)
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
	if elapsed, ok := s.closeSpan(err); ok {
		s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
	}
}

// EndWithoutObservation closes the stage span exactly as End does but
// makes no duration observation. It is for stages whose window is only
// comparable across runs when it completed, so a truncated window
// would skew the histogram while the span still needs to report the
// failure.
func (s *StageSpan) EndWithoutObservation(err error) {
	s.closeSpan(err)
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
	stage Stage,
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
	stage Stage,
	scope Scope,
	model StageModel,
	start, end time.Time,
	err error,
	attrs ...attribute.KeyValue,
) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		t.RecordAnomaly(StageAnomalyInvertedWindow)
		return
	}
	chatKind := chatKindFromContext(ctx)
	organization := organizationFromContext(ctx)
	_, span := t.otelTracer().Start(ctx, string(stage),
		trace.WithTimestamp(start),
		trace.WithAttributes(attrs...),
		trace.WithAttributes(model.attributes()...),
		trace.WithAttributes(stageIdentityAttributes(scope, chatKind, organization)...),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End(trace.WithTimestamp(end))
	t.observe(stage, scope, chatKind, model, end.Sub(start))
}

// RecordAnomaly counts a stage observation that was dropped or adjusted
// for reason. It is safe to call on a nil tracer.
func (t *StageTracer) RecordAnomaly(reason string) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageAnomaly(reason)
}

func (t *StageTracer) observe(stage Stage, scope Scope, chatKind ChatKind, model StageModel, elapsed time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageDuration(stage, scope, chatKind, model, elapsed)
}
