package recorder

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

var _ Recorder = &ValidatingRecorder{}

// ErrInvalidRecord reports a record that cannot be recorded as given. Callers
// receive it in place of a delegated call's error.
var ErrInvalidRecord = xerrors.New("invalid record")

// ValidatingRecorder refuses records whose timestamp is unset, so a record that
// would be persisted in year one, skewing budget periods, retention and the
// sessions timeline, fails where it was produced rather than after a round trip
// to coderd.
//
// Only timestamps are checked. Every other property of a record is the
// database's to enforce, so that this recorder cannot diverge from it.
//
// It belongs below the logging middleware, so that a refused record is logged
// with its payload, and above any middleware that drops records, so that a
// record's validity does not depend on the deployment's record policy.
//
// Refusal is not equally visible per record type: an interception is recorded
// synchronously and its error fails the intercepted request, while every other
// record is dispatched through [AsyncRecorder], which discards errors. This
// recorder therefore logs each refusal itself.
type ValidatingRecorder struct {
	logger  slog.Logger
	wrapped Recorder
}

// NewValidatingRecorder creates a [ValidatingRecorder] which delegates valid
// records to wrapped. wrapped may be nil, in which case valid records are
// discarded.
func NewValidatingRecorder(logger slog.Logger, wrapped Recorder) *ValidatingRecorder {
	return &ValidatingRecorder{logger: logger, wrapped: wrapped}
}

// WithValidation returns a [Middleware] which wraps a [Recorder] in a
// [ValidatingRecorder], for composition by [ChainMiddleware].
func WithValidation(logger slog.Logger) Middleware {
	return func(next Recorder) Recorder {
		return NewValidatingRecorder(logger, next)
	}
}

// refuse reports an unset timestamp. field names the timestamp, so that the
// reason is a stable, low cardinality string that can be alerted on. The
// interception's ID is already on the context of every call.
func (r *ValidatingRecorder) refuse(ctx context.Context, recordType, field string) error {
	r.logger.Error(ctx, "refusing to record record with unset timestamp",
		slog.F("record_type", recordType),
		slog.F("field", field),
	)
	return xerrors.Errorf("%s: %s is unset: %w", recordType, field, ErrInvalidRecord)
}

func (r *ValidatingRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	if req.StartedAt.IsZero() {
		return r.refuse(ctx, RecordTypeInterceptionStart, "started_at")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterception(ctx, req)
}

func (r *ValidatingRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	if req.EndedAt.IsZero() {
		return r.refuse(ctx, RecordTypeInterceptionEnd, "ended_at")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterceptionEnded(ctx, req)
}

func (r *ValidatingRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	if err := r.validateCreatedAt(ctx, RecordTypeTokenUsage, req.CreatedAt); err != nil {
		return err
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordTokenUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	if err := r.validateCreatedAt(ctx, RecordTypePromptUsage, req.CreatedAt); err != nil {
		return err
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordPromptUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	if err := r.validateCreatedAt(ctx, RecordTypeToolUsage, req.CreatedAt); err != nil {
		return err
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordToolUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	if err := r.validateCreatedAt(ctx, RecordTypeModelThought, req.CreatedAt); err != nil {
		return err
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordModelThought(ctx, req)
}

// validateCreatedAt applies the rule every record about an interception shares.
func (r *ValidatingRecorder) validateCreatedAt(ctx context.Context, recordType string, createdAt time.Time) error {
	if createdAt.IsZero() {
		return r.refuse(ctx, recordType, "created_at")
	}
	return nil
}
