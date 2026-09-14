package recorder

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

var _ Recorder = &LogRecorder{}

// LogRecorder optionally wraps another [Recorder] and logs every call.
// If the wrapped recorder is nil, calls are logged but not delegated on.
//
// LogRecorder is the single place where calls to a [Recorder] are logged.
// Logs were taken from [WrappedRecorder] and [AsyncRecorder] for backwards
// compatibility. A single failure therefore still produces two lines for
// most records. This is suboptimal, but a requirement for backwards compatibility.
type LogRecorder struct {
	logger  slog.Logger
	wrapped Recorder
}

// NewLogRecorder creates a [LogRecorder] which logs each record and then
// delegates it to wrapped. wrapped may be nil, in which case records are only
// logged.
func NewLogRecorder(logger slog.Logger, wrapped Recorder) *LogRecorder {
	return &LogRecorder{logger: logger, wrapped: wrapped}
}

// WithLogging returns a [Decorator] which wraps a [Recorder] in a
// [LogRecorder], for composition by [ChainMiddleware].
func WithLogging(logger slog.Logger) Middleware {
	return func(next Recorder) Recorder {
		return NewLogRecorder(logger, next)
	}
}

func (r *LogRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordInterception(ctx, req)
	}
	interceptionLogs := logEntry{
		success:              "recorded interception",
		wrappedRecorderError: "failed to record interception",
		fields: []slog.Field{
			slog.F("id", req.ID),
			slog.F("initiator_id", req.InitiatorID),
			slog.F("provider", req.Provider),
			slog.F("provider_name", req.ProviderName),
			slog.F("model", req.Model),
			slog.F("client", req.Client),
			slog.F("client_session_id", ptr.NilToEmpty(req.ClientSessionID)),
			slog.F("user_agent", req.UserAgent),
			slog.F("correlating_tool_call_id", ptr.NilToEmpty(req.CorrelatingToolCallID)),
			slog.F("credential_kind", req.CredentialKind),
			slog.F("credential_hint", req.CredentialHint),
			slog.F("agent_firewall_session_id", ptr.NilToEmpty(req.AgentFirewallSessionID)),
			slog.F("agent_firewall_sequence_number", ptr.NilToEmpty(req.AgentFirewallSequenceNumber)),
			slog.F("metadata", req.Metadata),
		},
	}
	return r.logResult(ctx, interceptionLogs, req, err)
}

func (r *LogRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordInterceptionEnded(ctx, req)
	}
	interceptionEndedLogs := logEntry{
		success:              "recorded interception ended",
		wrappedRecorderError: "failed to record that interception ended",
		asyncRecorderError:   "failed to record interception end",
		// [AsyncRecorder] labeled this record "prompt"; preserved as-is.
		typ: "prompt",
		fields: []slog.Field{
			slog.F("id", req.ID),
			slog.F("credential_hint", req.CredentialHint),
			slog.F("error_type", string(req.ErrorType)),
			slog.F("error_message", req.ErrorMessage),
		},
	}

	return r.logResult(ctx, interceptionEndedLogs, req, err)
}

func (r *LogRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordPromptUsage(ctx, req)
	}
	promptUsageLogs := logEntry{
		success:              "recorded prompt usage",
		wrappedRecorderError: "failed to record prompt usage",
		asyncRecorderError:   "failed to record usage",
		typ:                  "prompt",
		fields: []slog.Field{
			slog.F("interception_id", req.InterceptionID),
			slog.F("msg_id", req.MsgID),
			slog.F("prompt", req.Prompt),
			slog.F("metadata", req.Metadata),
		},
	}

	return r.logResult(ctx, promptUsageLogs, req, err)
}

func (r *LogRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordTokenUsage(ctx, req)
	}
	tokenUsageLogs := logEntry{
		success:              "recorded token usage",
		wrappedRecorderError: "failed to record token usage",
		asyncRecorderError:   "failed to record usage",
		typ:                  "token",
		fields: []slog.Field{
			slog.F("interception_id", req.InterceptionID),
			slog.F("msg_id", req.MsgID),
			slog.F("input_tokens", req.Input),
			slog.F("output_tokens", req.Output),
			slog.F("cache_read_input_tokens", req.CacheReadInputTokens),
			slog.F("cache_write_input_tokens", req.CacheWriteInputTokens),
			slog.F("extra_token_types", req.ExtraTokenTypes),
			slog.F("metadata", req.Metadata),
		},
	}

	return r.logResult(ctx, tokenUsageLogs, req, err)
}

func (r *LogRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordToolUsage(ctx, req)
	}
	var invocationErr string
	if req.InvocationError != nil {
		invocationErr = req.InvocationError.Error()
	}
	toolUsageLogs := logEntry{
		success:              "recorded tool usage",
		wrappedRecorderError: "failed to record tool usage",
		asyncRecorderError:   "failed to record usage",
		typ:                  "tool",
		fields: []slog.Field{
			slog.F("interception_id", req.InterceptionID),
			slog.F("msg_id", req.MsgID),
			slog.F("tool", req.Tool),
			slog.F("tool_call_id", req.ToolCallID),
			slog.F("item_id", req.ItemID),
			slog.F("server_url", ptr.NilToEmpty(req.ServerURL)),
			slog.F("args", req.Args),
			slog.F("injected", req.Injected),
			slog.F("invocation_error", invocationErr),
			slog.F("metadata", req.Metadata),
		},
	}
	return r.logResult(ctx, toolUsageLogs, req, err)
}

func (r *LogRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordModelThought(ctx, req)
	}
	modelThoughtLogs := logEntry{
		success:              "recorded model thought",
		wrappedRecorderError: "failed to record model thought",
		asyncRecorderError:   "failed to record model thought",
		typ:                  "model_thought",
		fields: []slog.Field{
			slog.F("interception_id", req.InterceptionID),
			slog.F("content", req.Content),
			slog.F("metadata", req.Metadata),
		},
	}

	return r.logResult(ctx, modelThoughtLogs, req, err)
}

// logEntry defines a standard format for the messages logged for each
// [Recorder] method.
type logEntry struct {
	// success is logged when the record was delegated successfully.
	success string
	// wrappedRecorderError is the failure message formerly logged by [WrappedRecorder].
	wrappedRecorderError string
	// asyncRecorderError is the failure message formerly logged by [AsyncRecorder]. It is
	// empty for records which never passed through [AsyncRecorder].
	asyncRecorderError string
	// typ is the "type" field which accompanied generic.
	typ    string
	fields []slog.Field
}

// logResult centralizes logging done throughout the [LogRecorder] for consistency.
// [LogRecorder] takes over the logging responsibility from both [WrappedRecorder]
// and [AsyncRecorder]. To preserve the inherited behavior, it logs two similar
// lines for every call. This duplicate logging is suboptimal, but also load bearing.
// It cannot be easily deduplicated, because customers have had the opportunity to
// build observability and alerting based on it.
func (r *LogRecorder) logResult(ctx context.Context, logs logEntry, payload any, err error) error {
	if err == nil {
		r.logger.Debug(ctx, logs.success, logs.fields...)
		return nil
	}

	r.logger.Warn(ctx, logs.wrappedRecorderError, slog.Error(err))
	if logs.asyncRecorderError != "" {
		r.logger.Warn(ctx, logs.asyncRecorderError, slog.F("type", logs.typ), slog.Error(err), slog.F("payload", payload))
	}
	return err
}
