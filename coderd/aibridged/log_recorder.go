package aibridged

import (
	"context"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

var _ aibridge.Recorder = &LogRecorder{}

// LogRecorder logs every record before delegating it to a wrapped [aibridge.Recorder].
// If the wrapped recorder is nil, records are only logged.
type LogRecorder struct {
	logger  slog.Logger
	wrapped aibridge.Recorder
}

// NewLogRecorder creates a [LogRecorder] which logs each call and then delegates
// it to wrapped. wrapped may be nil, in which case calls are only logged.
func NewLogRecorder(logger slog.Logger, wrapped aibridge.Recorder) *LogRecorder {
	return &LogRecorder{logger: logger, wrapped: wrapped}
}

func (r *LogRecorder) RecordInterception(ctx context.Context, req *aibridge.InterceptionRecord) error {
	r.logger.Info(ctx, "record interception",
		slog.F("id", req.ID),
		slog.F("initiator_id", req.InitiatorID),
		slog.F("provider", req.Provider),
		slog.F("provider_name", req.ProviderName),
		slog.F("model", req.Model),
		slog.F("client", req.Client),
		slog.F("client_session_id", ptr.NilToEmpty(req.ClientSessionID)),
		slog.F("user_agent", req.UserAgent),
		slog.F("started_at", req.StartedAt),
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
	return r.logResult(ctx, "interception", err, slog.F("id", req.ID))
}

func (r *LogRecorder) RecordInterceptionEnded(ctx context.Context, req *aibridge.InterceptionRecordEnded) error {
	r.logger.Info(ctx, "record interception ended",
		slog.F("id", req.ID),
		slog.F("ended_at", req.EndedAt),
		slog.F("credential_hint", req.CredentialHint),
		slog.F("error_type", string(req.ErrorType)),
		slog.F("error_message", req.ErrorMessage),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordInterceptionEnded(ctx, req)
	return r.logResult(ctx, "interception ended", err, slog.F("id", req.ID))
}

func (r *LogRecorder) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRecord) error {
	r.logger.Info(ctx, "record prompt usage",
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("prompt", req.Prompt),
		slog.F("metadata", req.Metadata),
		slog.F("created_at", req.CreatedAt),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordPromptUsage(ctx, req)
	return r.logResult(ctx, "prompt usage", err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRecord) error {
	r.logger.Info(ctx, "record token usage",
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("input_tokens", req.Input),
		slog.F("output_tokens", req.Output),
		slog.F("cache_read_input_tokens", req.CacheReadInputTokens),
		slog.F("cache_write_input_tokens", req.CacheWriteInputTokens),
		slog.F("extra_token_types", req.ExtraTokenTypes),
		slog.F("metadata", req.Metadata),
		slog.F("created_at", req.CreatedAt),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordTokenUsage(ctx, req)
	return r.logResult(ctx, "token usage", err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRecord) error {
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
		slog.F("created_at", req.CreatedAt),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordToolUsage(ctx, req)
	return r.logResult(ctx, "tool usage", err, slog.F("interception_id", req.InterceptionID))
}

func (r *LogRecorder) RecordModelThought(ctx context.Context, req *aibridge.ModelThoughtRecord) error {
	r.logger.Info(ctx, "record model thought",
		slog.F("interception_id", req.InterceptionID),
		slog.F("content", req.Content),
		slog.F("metadata", req.Metadata),
		slog.F("created_at", req.CreatedAt),
	)

	if r.wrapped == nil {
		return nil
	}
	err := r.wrapped.RecordModelThought(ctx, req)
	return r.logResult(ctx, "model thought", err, slog.F("interception_id", req.InterceptionID))
}

// logResult logs the outcome of a record delegated to the wrapped recorder, and
// returns err unchanged.
func (r *LogRecorder) logResult(ctx context.Context, name string, err error, fields ...slog.Field) error {
	if err != nil {
		r.logger.Error(ctx, "failed to record "+name, append(fields, slog.Error(err))...)
		return err
	}

	r.logger.Debug(ctx, "recorded "+name, fields...)
	return nil
}
