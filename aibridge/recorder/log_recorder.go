package recorder

import (
	"context"
	"encoding/json"
	"maps"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

var _ Recorder = &LogRecorder{}

// The structured log contract for AI Gateway interception records. Customer
// SIEM pipelines select and parse on these values, so they are public API and
// must not change without a deprecation. They live here, rather than beside
// either emitter, because both the gateway and coderd emit this format and
// duplicated literals would let the two drift.
const (
	InterceptionLogMarker = "interception log"
	MetadataUserAgentKey  = "request_user_agent"
)

// Values of the record_type field, one per [Recorder] method.
const (
	RecordTypeInterceptionStart = "interception_start"
	RecordTypeInterceptionEnd   = "interception_end"
	RecordTypeTokenUsage        = "token_usage"
	RecordTypePromptUsage       = "prompt_usage"
	RecordTypeToolUsage         = "tool_usage"
	RecordTypeModelThought      = "model_thought"
)

// LogRecorder optionally wraps another [Recorder] and logs every call.
// If the wrapped recorder is nil, calls are logged but not delegated on.
//
// LogRecorder is the single place where calls to a [Recorder] are logged.
// Logs were taken from [WrappedRecorder] and [AsyncRecorder] for backwards
// compatibility. A single failure therefore still produces two lines for
// most records. This is suboptimal, but a requirement for backwards compatibility.
type LogRecorder struct {
	logger     slog.Logger
	apiKeyID   string
	structured bool
	wrapped    Recorder
}

// NewLogRecorder creates a [LogRecorder] which logs each record and then
// delegates it to wrapped. wrapped may be nil, in which case records are only
// logged.
func NewLogRecorder(logger slog.Logger, apiKeyID string, structured bool, wrapped Recorder) *LogRecorder {
	return &LogRecorder{logger: logger, apiKeyID: apiKeyID, structured: structured, wrapped: wrapped}
}

// WithLogging returns a [Middleware] which wraps a [Recorder] in a
// [LogRecorder], for composition by [ChainMiddleware].
func WithLogging(logger slog.Logger, apiKeyID string, structured bool) Middleware {
	return func(next Recorder) Recorder {
		return NewLogRecorder(logger, apiKeyID, structured, next)
	}
}

// logStructured emits one record in the format described by
// [InterceptionLogMarker], when the deployment has asked for it.
//
// It runs before the record is delegated, so that a record dropped lower in the
// chain is still reported.
func (r *LogRecorder) logStructured(ctx context.Context, recordType string, fields ...slog.Field) {
	if !r.structured {
		return
	}
	r.logger.Info(ctx, InterceptionLogMarker, append([]slog.Field{slog.F("record_type", recordType)}, fields...)...)
}

// metadataWithUserAgent copies metadata rather than writing through: the
// record's own metadata travels on to coderd, which merges the user agent
// itself and warns when the key is already set.
func metadataWithUserAgent(metadata Metadata, userAgent string) Metadata {
	if userAgent == "" {
		return metadata
	}
	merged := make(Metadata, len(metadata)+1)
	maps.Copy(merged, metadata)
	merged[MetadataUserAgentKey] = userAgent
	return merged
}

// marshalToolArgs renders tool call arguments the way [DRPCRecorder] sends
// them, so that both emitters report an identical input field.
func (r *LogRecorder) marshalToolArgs(ctx context.Context, args ToolArgs) string {
	serialized, err := json.Marshal(args)
	if err != nil {
		// An empty input field is indistinguishable from a tool called with
		// no arguments, so the difference is reported here.
		r.logger.Warn(ctx, "failed to marshal tool call arguments, reporting empty input", slog.Error(err))
		return ""
	}
	return string(serialized)
}

func (r *LogRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	// thread_parent_id and thread_root_id are deliberately absent: they are
	// resolved from recorded tool usages by a database lookup which only
	// coderd can perform. An absent key fails a consumer loudly; a nil UUID
	// would assert, wrongly, that the interception has no parent.
	r.logStructured(ctx, RecordTypeInterceptionStart,
		slog.F("interception_id", req.ID),
		slog.F("initiator_id", req.InitiatorID),
		slog.F("api_key_id", r.apiKeyID),
		slog.F("provider", req.Provider),
		slog.F("model", req.Model),
		slog.F("client", req.Client),
		slog.F("client_session_id", ptr.NilToEmpty(req.ClientSessionID)),
		slog.F("started_at", req.StartedAt),
		slog.F("metadata", metadataWithUserAgent(req.Metadata, req.UserAgent)),
		slog.F("correlating_tool_call_id", ptr.NilToEmpty(req.CorrelatingToolCallID)),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordInterception(ctx, req)
	}
	interceptionLogs := logEntry{
		success:       "recorded interception",
		failureDetail: "failed to record interception",
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
	r.logStructured(ctx, RecordTypeInterceptionEnd,
		slog.F("interception_id", req.ID),
		slog.F("ended_at", req.EndedAt),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordInterceptionEnded(ctx, req)
	}
	interceptionEndedLogs := logEntry{
		success:       "recorded interception ended",
		failure:       "failed to record interception end",
		failureDetail: "failed to record that interception ended",
		typ:           "prompt",
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
	r.logStructured(ctx, RecordTypePromptUsage,
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("prompt", req.Prompt),
		slog.F("created_at", req.CreatedAt),
		slog.F("metadata", req.Metadata),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordPromptUsage(ctx, req)
	}
	promptUsageLogs := logEntry{
		success:       "recorded prompt usage",
		failure:       "failed to record usage",
		failureDetail: "failed to record prompt usage",
		typ:           "prompt",
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
	r.logStructured(ctx, RecordTypeTokenUsage,
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("input_tokens", req.Input),
		slog.F("output_tokens", req.Output),
		slog.F("cache_read_input_tokens", req.CacheReadInputTokens),
		slog.F("cache_write_input_tokens", req.CacheWriteInputTokens),
		slog.F("created_at", req.CreatedAt),
		slog.F("metadata", req.Metadata),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordTokenUsage(ctx, req)
	}
	tokenUsageLogs := logEntry{
		success:       "recorded token usage",
		failure:       "failed to record usage",
		failureDetail: "failed to record token usage",
		typ:           "token",
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
	var structuredInvocationErr string
	if req.InvocationError != nil {
		structuredInvocationErr = req.InvocationError.Error()
	}
	r.logStructured(ctx, RecordTypeToolUsage,
		slog.F("interception_id", req.InterceptionID),
		slog.F("msg_id", req.MsgID),
		slog.F("tool_call_id", req.ToolCallID),
		slog.F("item_id", req.ItemID),
		slog.F("tool", req.Tool),
		slog.F("input", r.marshalToolArgs(ctx, req.Args)),
		slog.F("server_url", ptr.NilToEmpty(req.ServerURL)),
		slog.F("injected", req.Injected),
		slog.F("invocation_error", structuredInvocationErr),
		slog.F("created_at", req.CreatedAt),
		slog.F("metadata", req.Metadata),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordToolUsage(ctx, req)
	}
	var invocationErr string
	if req.InvocationError != nil {
		invocationErr = req.InvocationError.Error()
	}
	toolUsageLogs := logEntry{
		success:       "recorded tool usage",
		failure:       "failed to record usage",
		failureDetail: "failed to record tool usage",
		typ:           "tool",
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
	r.logStructured(ctx, RecordTypeModelThought,
		slog.F("interception_id", req.InterceptionID),
		slog.F("content", req.Content),
		slog.F("created_at", req.CreatedAt),
		slog.F("metadata", req.Metadata),
	)

	var err error
	if r.wrapped != nil {
		err = r.wrapped.RecordModelThought(ctx, req)
	}
	modelThoughtLogs := logEntry{
		success:       "recorded model thought",
		failure:       "failed to record model thought",
		failureDetail: "failed to record model thought",
		typ:           "model_thought",
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
	success       string
	failure       string
	failureDetail string
	typ           string
	fields        []slog.Field
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

	r.logger.Warn(ctx, logs.failureDetail, slog.Error(err))
	if logs.failure != "" {
		r.logger.Warn(ctx, logs.failure, slog.F("type", logs.typ), slog.Error(err), slog.F("payload", payload))
	}
	return err
}
