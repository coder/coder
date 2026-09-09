package recorder

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

var _ Recorder = &LogRecorder{}

// LogRecorder logs every record before delegating it to a wrapped [Recorder],
// then logs the outcome of that delegation. If the wrapped recorder is nil,
// records are only logged.
//
// It is the single place where records are logged. The failure messages it
// emits are reproduced verbatim from [WrappedRecorder] and [AsyncRecorder],
// which used to log them from two different positions in the recorder chain,
// so log-based alerting on either message keeps working. A single failure
// therefore still produces two lines for most records; see [resultLogs].
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

// resultLogs holds the messages logged for one record type once delegation has
// completed.
type resultLogs struct {
	// success is logged when the record was delegated successfully.
	success string
	// specific is the failure message formerly logged by [WrappedRecorder].
	specific string
	// generic is the failure message formerly logged by [AsyncRecorder]. It is
	// empty for records which never passed through [AsyncRecorder].
	generic string
	// typ is the "type" field which accompanied generic.
	typ string
}

var (
	interceptionLogs = resultLogs{
		success:  "recorded interception",
		specific: "failed to record interception",
		// [AsyncRecorder] never handled interceptions; recording one must not
		// be deferred, since a failure fails the whole request.
	}
	interceptionEndedLogs = resultLogs{
		success:  "recorded interception ended",
		specific: "failed to record that interception ended",
		generic:  "failed to record interception end",
		// [AsyncRecorder] labeled this record "prompt"; preserved as-is.
		typ: "prompt",
	}
	promptUsageLogs = resultLogs{
		success:  "recorded prompt usage",
		specific: "failed to record prompt usage",
		generic:  "failed to record usage",
		typ:      "prompt",
	}
	tokenUsageLogs = resultLogs{
		success:  "recorded token usage",
		specific: "failed to record token usage",
		generic:  "failed to record usage",
		typ:      "token",
	}
	toolUsageLogs = resultLogs{
		success:  "recorded tool usage",
		specific: "failed to record tool usage",
		generic:  "failed to record usage",
		typ:      "tool",
	}
	modelThoughtLogs = resultLogs{
		success:  "recorded model thought",
		specific: "failed to record model thought",
		generic:  "failed to record model thought",
		typ:      "model_thought",
	}
)

// logResult logs the outcome of delegating a record to the wrapped recorder and
// returns err unchanged. payload is the record itself, logged with the failure
// message which [AsyncRecorder] used to emit.
func (r *LogRecorder) logResult(ctx context.Context, logs resultLogs, payload any, err error, fields ...slog.Field) error {
	if err == nil {
		r.logger.Debug(ctx, logs.success, fields...)
		return nil
	}

	r.logger.Warn(ctx, logs.specific, slog.Error(err))
	if logs.generic != "" {
		r.logger.Warn(ctx, logs.generic, slog.F("type", logs.typ), slog.Error(err), slog.F("payload", payload))
	}
	return err
}

func (r *LogRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	r.logger.Info(ctx, "record interception",
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
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordInterception(ctx, req)
	return r.logResult(ctx, interceptionLogs, req, err, slog.F("id", req.ID))
}

func (r *LogRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	r.logger.Info(ctx, "record interception ended",
		slog.F("id", req.ID),
		slog.F("credential_hint", req.CredentialHint),
		slog.F("error_type", string(req.ErrorType)),
		slog.F("error_message", req.ErrorMessage),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordInterceptionEnded(ctx, req)
	return r.logResult(ctx, interceptionEndedLogs, req, err, slog.F("id", req.ID))
}

func (r *LogRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	r.logger.Info(ctx, "record prompt usage",
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("prompt", req.Prompt),
		slog.F("metadata", req.Metadata),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordPromptUsage(ctx, req)
	return r.logResult(ctx, promptUsageLogs, req, err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	r.logger.Info(ctx, "record token usage",
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("input_tokens", req.Input),
		slog.F("output_tokens", req.Output),
		slog.F("cache_read_input_tokens", req.CacheReadInputTokens),
		slog.F("cache_write_input_tokens", req.CacheWriteInputTokens),
		slog.F("extra_token_types", req.ExtraTokenTypes),
		slog.F("metadata", req.Metadata),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordTokenUsage(ctx, req)
	return r.logResult(ctx, tokenUsageLogs, req, err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	var invocationErr string
	if req.InvocationError != nil {
		invocationErr = req.InvocationError.Error()
	}

	r.logger.Info(ctx, "record tool usage",
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
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordToolUsage(ctx, req)
	return r.logResult(ctx, toolUsageLogs, req, err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	r.logger.Info(ctx, "record model thought",
		slog.F("interception_id", req.InterceptionID),
		slog.F("content", req.Content),
		slog.F("metadata", req.Metadata),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordModelThought(ctx, req)
	return r.logResult(ctx, modelThoughtLogs, req, err, slog.F("interception_id", req.InterceptionID))
}
