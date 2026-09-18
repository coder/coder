package recorder

import "context"

var _ Recorder = &FilterRecorder{}

// DisabledRecords selects the records a [FilterRecorder] drops.
type DisabledRecords struct {
	Interception      bool
	InterceptionEnded bool
	TokenUsage        bool
	PromptUsage       bool
	ToolUsage         bool
	ModelThought      bool
}

func (d DisabledRecords) Disabled() bool {
	return d.Interception && d.InterceptionEnded && d.TokenUsage &&
		d.PromptUsage && d.ToolUsage && d.ModelThought
}

// FilterRecorder wraps another [Recorder] and drops the records selected by a
// [DisabledRecords], so that a deployment can keep some record types out of its
// database while the rest of the chain keeps working.
//
// A dropped record is reported to the caller as recorded successfully. The
// caller produced the record before the deployment's choice not to keep it was
// known, and cannot act on that choice, so failing the call would fail the
// intercepted request for a record the deployment did not want.
//
// It belongs below the [LogRecorder], so that a dropped record is still logged.
// Note that this means a record can reach the logs without reaching the
// database; see [LogRecorder], which logs each record's payload. Anything below
// FilterRecorder, including the spans of an enclosing [TraceRecorder], never
// observes a dropped record.
type FilterRecorder struct {
	disabled DisabledRecords
	wrapped  Recorder
}

// NewFilterRecorder creates a [FilterRecorder]. wrapped may be nil, in which
// case every record is dropped.
func NewFilterRecorder(disabled DisabledRecords, wrapped Recorder) *FilterRecorder {
	return &FilterRecorder{disabled: disabled, wrapped: wrapped}
}

// WithoutRecords returns a [Middleware] which wraps a [Recorder] in a
// [FilterRecorder], for composition by [ChainMiddleware].
func WithoutRecords(disabled DisabledRecords) Middleware {
	return func(next Recorder) Recorder {
		return NewFilterRecorder(disabled, next)
	}
}

func (r *FilterRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	if r.disabled.Interception || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterception(ctx, req)
}

func (r *FilterRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	if r.disabled.InterceptionEnded || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterceptionEnded(ctx, req)
}

func (r *FilterRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	if r.disabled.TokenUsage || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordTokenUsage(ctx, req)
}

func (r *FilterRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	if r.disabled.PromptUsage || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordPromptUsage(ctx, req)
}

func (r *FilterRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	if r.disabled.ToolUsage || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordToolUsage(ctx, req)
}

func (r *FilterRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	if r.disabled.ModelThought || r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordModelThought(ctx, req)
}
