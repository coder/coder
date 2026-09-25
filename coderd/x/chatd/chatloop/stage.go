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

// Stage names a chat lifecycle stage. Every value is a span name;
// values in observedStages are also `stage` label values.
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
	AttrCompactionSource  = "compaction_source"
	AttrScope             = "scope"
)

// Scope is the `scope` label and span attribute of a stage. A stage is
// turn scoped when its latency is attributable to a prompt: it runs
// inside a chat turn's trace, or it is a wait such as queue_wait that
// is recorded before the prompt's turn opens. A stage is background
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

// StageTracer emits a span per chat lifecycle stage and, for stages in
// observedStages, a duration observation computed from the same start
// and end. Live spans take their timestamps and durations from the
// tracer's clock; Record uses the caller's timestamps for both.
//
// A nil *StageTracer is usable and discards everything.
type StageTracer struct {
	tracer  trace.Tracer
	metrics *Metrics
	clock   quartz.Clock
}

// StageTracerOption configures a StageTracer.
type StageTracerOption func(*StageTracer)

// WithClock sets the clock the tracer reads stage boundaries from.
func WithClock(clock quartz.Clock) StageTracerOption {
	return func(t *StageTracer) {
		t.clock = clock
	}
}

// NewStageTracer builds a stage tracer. A nil provider discards spans
// and nil metrics discards observations.
func NewStageTracer(provider trace.TracerProvider, metrics *Metrics, opts ...StageTracerOption) *StageTracer {
	if provider == nil {
		provider = noop.NewTracerProvider()
	}
	if metrics == nil {
		metrics = NopMetrics()
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
// queue wait. Provider is the wire protocol the model client speaks
// (Model.Provider()). ProviderType is the configured type of the AI
// provider the model config points at, such as "bedrock"; it differs
// from Provider for Bedrock and the OpenAI-compatible provider types.
// Effort is the Coder-scale reasoning effort resolved from the request
// and the model config's default and max, before provider-specific
// mapping; it is empty when none resolves.
type StageModel struct {
	Provider     string
	ProviderType string
	Model        string
	Effort       string
}

// attributes returns the span attributes for the identity, omitting
// the ones that are unknown.
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

// StageSpan is an in-flight stage. Close it with End, which records
// the duration, or EndWithoutObservation, which does not; calls after
// the first are ignored. A StageSpan is not safe for concurrent use.
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

// StartRootAt begins a stage span in its own trace, ignoring any span
// in ctx, that started at an earlier, already known instant. The span
// opens a turn, so it is turn scoped regardless of what ctx carries.
// The span timestamp and the recorded duration both run from start,
// so stages reconstructed inside the span still fall within it. A zero
// start means the span begins now. A start after the tracer's current
// time is replaced by now and, for observed stages, counted as a
// future_start anomaly.
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
		// A start ahead of this replica's clock was stamped by another
		// host; the span begins now so its window is not negative.
		t.recordAnomalyIfObserved(stage, StageAnomalyFutureStart)
		start = now
	}
	opts = append(opts, trace.WithTimestamp(start))
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
// carries; an unknown chat kind or organization is omitted.
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

// EndWithoutObservation closes the span like End but records no
// duration, for stages whose truncated window would skew the
// histogram.
func (s *StageSpan) EndWithoutObservation(err error) {
	s.closeSpan(err)
}

// closeSpan ends the span and returns its window; ok is false for a
// nil span and for calls after the first.
func (s *StageSpan) closeSpan(err error) (elapsed time.Duration, ok bool) {
	if s == nil || s.ended {
		return 0, false
	}
	s.ended = true
	end := s.tracer.Now()
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End(trace.WithTimestamp(end))
	return end.Sub(s.start), true
}

// Record emits an already-finished stage span with explicit start and
// end timestamps. It is for stages whose boundaries are only known
// after the fact, such as durations reconstructed from persisted
// timestamps. The stage takes the scope and chat kind on ctx.
// Windows with an unset timestamp or an end before the start are
// dropped and, for observed stages, counted as anomalies; a zero-width
// window is observed.
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
func (t *StageTracer) RecordAnomaly(reason StageAnomaly) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageAnomaly(reason)
}

// recordAnomalyIfObserved counts reason only when stage is observed,
// since span-only stages have no observation to drop or adjust.
func (t *StageTracer) recordAnomalyIfObserved(stage Stage, reason StageAnomaly) {
	if _, ok := observedStages[stage]; !ok {
		return
	}
	t.RecordAnomaly(reason)
}

func (t *StageTracer) observe(stage Stage, scope Scope, chatKind ChatKind, model StageModel, elapsed time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageDuration(stage, scope, chatKind, model, elapsed)
}
