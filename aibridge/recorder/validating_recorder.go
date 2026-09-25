package recorder

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

var _ Recorder = &ValidatingRecorder{}

// ErrInvalidRecord reports a record that cannot be recorded as given. Callers
// receive it in place of a delegated call's error.
var ErrInvalidRecord = xerrors.New("invalid record")

// MaxTokenUsage bounds the token count a single record may report. coderd
// enforces the same bound on arrival, since it accepts records from gateways it
// does not control.
const MaxTokenUsage int64 = 1_000_000_000_000

// ValidatingRecorder rejects malformed records instead of delegating them, so a
// record that would corrupt the deployment's ledger fails where it was produced
// rather than after a round trip to coderd.
//
// It belongs below the logging middleware, so that a rejected record is logged
// with its payload, and above any middleware that drops records, so that a
// record's validity does not depend on the deployment's record policy.
//
// Rejection is not equally visible per record type: an interception is recorded
// synchronously and its error fails the intercepted request, while every other
// record is dispatched through [AsyncRecorder], which discards errors. This
// recorder therefore logs each rejection itself.
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

// reject reports why a record was refused. The reason is a stable, low
// cardinality string so that it can be alerted on. The interception's ID is
// already on the context of every call, so only an unusable value is reported
// here, as invalid_value.
func (r *ValidatingRecorder) reject(ctx context.Context, recordType, reason string, fields ...slog.Field) error {
	r.logger.Error(ctx, "refusing to record invalid record",
		append([]slog.Field{
			slog.F("record_type", recordType),
			slog.F("reason", reason),
		}, fields...)...)
	return xerrors.Errorf("%s: %s: %w", recordType, reason, ErrInvalidRecord)
}

// warn reports a record that is recorded despite being questionable. These
// cases are known to occur, so dropping the record would lose real data.
func (r *ValidatingRecorder) warn(ctx context.Context, recordType, reason string, fields ...slog.Field) {
	r.logger.Warn(ctx, "recording questionable record",
		append([]slog.Field{
			slog.F("record_type", recordType),
			slog.F("reason", reason),
		}, fields...)...)
}

func (r *ValidatingRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	if _, err := uuid.Parse(req.ID); err != nil {
		return r.reject(ctx, RecordTypeInterceptionStart, "interception_id is not a UUID", slog.F("invalid_value", req.ID))
	}
	if _, err := uuid.Parse(req.InitiatorID); err != nil {
		return r.reject(ctx, RecordTypeInterceptionStart, "initiator_id is not a UUID", slog.F("invalid_value", req.InitiatorID))
	}
	// Provider is set by the gateway's own routing, so an empty one is a bug
	// in the gateway rather than a bad request.
	if req.Provider == "" {
		return r.reject(ctx, RecordTypeInterceptionStart, "provider is empty")
	}
	if req.StartedAt.IsZero() {
		return r.reject(ctx, RecordTypeInterceptionStart, "started_at is unset")
	}
	// The model comes from the client's request body, and the upstream provider
	// is the authority on whether that body is acceptable. Refusing the record
	// would answer a malformed request with a gateway error instead of the
	// provider's own, so it is reported and recorded. Prices are keyed on
	// provider and model, so such an interception cannot be costed.
	if req.Model == "" {
		r.warn(ctx, RecordTypeInterceptionStart, "model is empty")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterception(ctx, req)
}

func (r *ValidatingRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	if _, err := uuid.Parse(req.ID); err != nil {
		return r.reject(ctx, RecordTypeInterceptionEnd, "interception_id is not a UUID", slog.F("invalid_value", req.ID))
	}
	if req.EndedAt.IsZero() {
		return r.reject(ctx, RecordTypeInterceptionEnd, "ended_at is unset")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordInterceptionEnded(ctx, req)
}

func (r *ValidatingRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	if err := r.validateUsage(ctx, RecordTypeTokenUsage, req.InterceptionID, req.MsgID, req.CreatedAt); err != nil {
		return err
	}
	for _, count := range []struct {
		name  string
		value int64
	}{
		{"input_tokens", req.Input},
		{"output_tokens", req.Output},
		{"cache_read_input_tokens", req.CacheReadInputTokens},
		{"cache_write_input_tokens", req.CacheWriteInputTokens},
	} {
		if count.value < 0 || count.value > MaxTokenUsage {
			return r.reject(ctx, RecordTypeTokenUsage, "token count out of range",
				slog.F("count_name", count.name),
				slog.F("count", count.value))
		}
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordTokenUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	if err := r.validateUsage(ctx, RecordTypePromptUsage, req.InterceptionID, req.MsgID, req.CreatedAt); err != nil {
		return err
	}
	// Empty prompts are produced today, so they are reported rather than
	// dropped; see the prompt metric guard in [AsyncRecorder].
	if req.Prompt == "" {
		r.warn(ctx, RecordTypePromptUsage, "prompt is empty")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordPromptUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	if err := r.validateUsage(ctx, RecordTypeToolUsage, req.InterceptionID, req.MsgID, req.CreatedAt); err != nil {
		return err
	}
	if req.Tool == "" {
		return r.reject(ctx, RecordTypeToolUsage, "tool is empty")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordToolUsage(ctx, req)
}

func (r *ValidatingRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	if err := r.validateUsage(ctx, RecordTypeModelThought, req.InterceptionID, "", req.CreatedAt); err != nil {
		return err
	}
	if req.Content == "" {
		r.warn(ctx, RecordTypeModelThought, "content is empty")
	}

	if r.wrapped == nil {
		return nil
	}
	return r.wrapped.RecordModelThought(ctx, req)
}

// validateUsage applies the rules every record about an interception shares.
// msgID is reported when empty rather than refused: it degrades correlation
// between the records of one model response, but loses nothing already
// recorded. Callers with no message ID pass an empty one and skip that report.
func (r *ValidatingRecorder) validateUsage(ctx context.Context, recordType, interceptionID, msgID string, createdAt time.Time) error {
	if _, err := uuid.Parse(interceptionID); err != nil {
		return r.reject(ctx, recordType, "interception_id is not a UUID", slog.F("invalid_value", interceptionID))
	}
	if createdAt.IsZero() {
		return r.reject(ctx, recordType, "created_at is unset")
	}
	if recordType != RecordTypeModelThought && msgID == "" {
		r.warn(ctx, recordType, "msg_id is empty")
	}
	return nil
}
