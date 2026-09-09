package recorder

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/aibridge/tracing"
)

var _ Recorder = &TraceRecorder{}

// Span names, preserved verbatim from [WrappedRecorder], which traced records
// before tracing was consolidated here. Should records ever be traced at more
// than one position in the chain, give [TraceRecorder] a name prefix per
// position rather than reusing these names.
const (
	spanRecordInterception      = "Intercept.RecordInterception"
	spanRecordInterceptionEnded = "Intercept.RecordInterceptionEnded"
	spanRecordPromptUsage       = "Intercept.RecordPromptUsage"
	spanRecordTokenUsage        = "Intercept.RecordTokenUsage"
	spanRecordToolUsage         = "Intercept.RecordToolUsage"
	spanRecordModelThought      = "Intercept.RecordModelThought"
)

// TraceRecorder starts a span around each record before delegating it to a
// wrapped [Recorder]. If the wrapped recorder is nil, only the span is
// recorded.
//
// It is the single place where records are traced. It belongs immediately above
// the recorder whose work the spans measure, so that they do not also cover the
// logging done by an enclosing [LogRecorder].
type TraceRecorder struct {
	tracer  trace.Tracer
	wrapped Recorder
}

// NewTraceRecorder creates a [TraceRecorder] which traces each record and then
// delegates it to wrapped. wrapped may be nil, in which case records are only
// traced.
func NewTraceRecorder(tracer trace.Tracer, wrapped Recorder) *TraceRecorder {
	return &TraceRecorder{tracer: tracer, wrapped: wrapped}
}

// WithTracing returns a [Decorator] which wraps a [Recorder] in a
// [TraceRecorder], for composition by [Chain]. As [TraceRecorder] documents, it
// belongs last in a chain, immediately above the recorder whose work its spans
// measure.
func WithTracing(tracer trace.Tracer) Decorator {
	return func(next Recorder) Recorder {
		return NewTraceRecorder(tracer, next)
	}
}

// start begins a span named name, carrying the interception attributes held by
// ctx, and returns the context the record must be delegated with.
func (r *TraceRecorder) start(ctx context.Context, name string) (context.Context, trace.Span) {
	return r.tracer.Start(ctx, name, trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
}

func (r *TraceRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) (outErr error) {
	ctx, span := r.start(ctx, spanRecordInterception)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterception(ctx, req)
}

func (r *TraceRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) (outErr error) {
	ctx, span := r.start(ctx, spanRecordInterceptionEnded)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterceptionEnded(ctx, req)
}

func (r *TraceRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) (outErr error) {
	ctx, span := r.start(ctx, spanRecordPromptUsage)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordPromptUsage(ctx, req)
}

func (r *TraceRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) (outErr error) {
	ctx, span := r.start(ctx, spanRecordTokenUsage)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordTokenUsage(ctx, req)
}

func (r *TraceRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) (outErr error) {
	ctx, span := r.start(ctx, spanRecordToolUsage)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordToolUsage(ctx, req)
}

func (r *TraceRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) (outErr error) {
	ctx, span := r.start(ctx, spanRecordModelThought)
	defer tracing.EndSpanErr(span, &outErr)

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordModelThought(ctx, req)
}
