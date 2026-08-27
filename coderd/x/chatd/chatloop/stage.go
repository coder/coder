package chatloop

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
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
)

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
// from the turn, such as title and summary generation that outlives
// the turn that triggered it.
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
	tracer *StageTracer
	stage  string
	scope  string
	model  StageModel
	span   trace.Span
	start  time.Time
	ended  bool
}

// scopeFromContext classifies work by whether ctx still carries the
// turn's trace. Detached background work has no span in its context,
// so its stages are kept out of the turn profile.
func scopeFromContext(ctx context.Context) string {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ScopeTurn
	}
	return ScopeBackground
}

// Start begins a stage span as a child of the span in ctx and returns
// a context carrying it. The stage is scoped by the span already in
// ctx, so stages started on a detached context are background scoped.
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
	now := time.Now()
	if start.IsZero() || start.After(now) {
		start = now
	} else {
		opts = append(opts, trace.WithTimestamp(start))
	}
	opts = append(opts, trace.WithAttributes(attribute.String(AttrScope, scope)))
	ctx, span := t.otelTracer().Start(ctx, stage, opts...)
	return ctx, &StageSpan{
		tracer: t,
		stage:  stage,
		scope:  scope,
		span:   span,
		start:  start,
	}
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
// duration observation End makes. Stages that only learn the model
// after they start, such as a generation step that resolves it during
// preparation, call this once it is known.
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
	if s == nil || s.ended {
		return
	}
	s.ended = true
	elapsed := time.Since(s.start)
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
	s.tracer.observe(s.stage, s.scope, s.model, elapsed)
}

// Record emits an already-finished stage span with explicit start and
// end timestamps. It is for stages whose boundaries are only known
// after the fact, such as durations reconstructed from persisted
// timestamps. The stage is scoped by the span in ctx. Non-positive or
// unset windows are dropped.
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

// RecordAs is Record with an explicit scope, for stages that belong
// to a turn but are reconstructed outside its trace, such as the
// capacity wait an acquisition pass measures before the turn span
// exists.
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
		return
	}
	_, span := t.otelTracer().Start(ctx, stage,
		trace.WithTimestamp(start),
		trace.WithAttributes(attrs...),
		trace.WithAttributes(model.attributes()...),
		trace.WithAttributes(attribute.String(AttrScope, scope)),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End(trace.WithTimestamp(end))
	t.observe(stage, scope, model, end.Sub(start))
}

func (t *StageTracer) observe(stage, scope string, model StageModel, elapsed time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordStageDuration(stage, scope, model.Model, model.Effort, elapsed)
}
