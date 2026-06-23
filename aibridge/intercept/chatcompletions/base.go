package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/quartz"
)

type interceptionBase struct {
	id  uuid.UUID
	req *ChatCompletionNewParamsWrapper

	cfg  intercept.Config
	cred intercept.Credential

	// clientHeaders are the original HTTP headers from the client request.
	clientHeaders http.Header

	logger slog.Logger
	tracer trace.Tracer

	recorder recorder.Recorder
	mcpProxy mcp.ServerProxier
}

// newCompletionsService builds the SDK service used for upstream calls.
func (i *interceptionBase) newCompletionsService(ctx context.Context) openai.ChatCompletionService {
	var opts []option.RequestOption
	// Only BYOK sets its credential here. Centralized keys are injected
	// per-attempt in the failover loop.
	if byok, ok := intercept.AsBYOK(i.cred); ok {
		i.logger.Debug(ctx, "using byok auth",
			slog.F("auth_header", byok.Header), slog.F("key_hint", byok.Hint()),
		)
		opts = append(opts, option.WithAPIKey(byok.Secret))
	}
	opts = append(opts, option.WithBaseURL(i.cfg.BaseURL))

	// Forward client headers to upstream. This middleware runs after the SDK
	// has built the request, and replaces the outgoing headers with the sanitized
	// client headers plus provider auth.
	if i.clientHeaders != nil {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			req.Header = intercept.BuildUpstreamHeaders(req.Header, i.clientHeaders, i.cred.AuthHeader())
			return next(req)
		}))
	}

	// Add API dump middleware if configured
	if mw := apidump.NewBridgeMiddleware(i.cfg.APIDumpDir, i.cfg.ProviderName, i.Model(), i.id, i.logger, quartz.NewReal()); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}

	return openai.NewChatCompletionService(opts...)
}

func (i *interceptionBase) ID() uuid.UUID {
	return i.id
}

func (i *interceptionBase) Credential() intercept.Credential {
	return i.cred
}

func (i *interceptionBase) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.logger = logger
	i.recorder = rec
	i.mcpProxy = mcpProxy
}

func (i *interceptionBase) CorrelatingToolCallID() *string {
	if len(i.req.Messages) == 0 {
		return nil
	}

	// The tool result should be the last input message.
	msg := i.req.Messages[len(i.req.Messages)-1]
	if msg.OfTool == nil {
		return nil
	}
	return &msg.OfTool.ToolCallID
}

func (i *interceptionBase) baseTraceAttributes(r *http.Request, streaming bool) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(tracing.RequestPath, r.URL.Path),
		attribute.String(tracing.InterceptionID, i.id.String()),
		attribute.String(tracing.InitiatorID, aibcontext.ActorIDFromContext(r.Context())),
		attribute.String(tracing.Provider, i.cfg.ProviderName),
		attribute.String(tracing.Model, i.Model()),
		attribute.Bool(tracing.Streaming, streaming),
	}
}

func (i *interceptionBase) Model() string {
	if i.req == nil {
		return "coder-aibridge-unknown"
	}

	return i.req.Model
}

func (*interceptionBase) newErrorResponse(err error) map[string]any {
	return map[string]any{
		"error":   true,
		"message": err.Error(),
	}
}

func (i *interceptionBase) injectTools() {
	if i.req == nil || i.mcpProxy == nil || !i.hasInjectableTools() {
		return
	}

	// Disable parallel tool calls when injectable tools are present to simplify the inner agentic loop.
	i.req.ParallelToolCalls = openai.Bool(false)

	// Inject tools.
	for _, tool := range i.mcpProxy.ListTools() {
		fn := openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        tool.ID,
					Strict:      openai.Bool(false), // TODO: configurable.
					Description: openai.String(tool.Description),
					Parameters: openai.FunctionParameters{
						"type":       "object",
						"properties": tool.Params,
						// "additionalProperties": false, // Only relevant when strict=true.
					},
				},
			},
		}

		// Otherwise the request fails with "None is not of type 'array'" if a nil slice is given.
		if len(tool.Required) > 0 {
			// Must list ALL properties when strict=true.
			fn.OfFunction.Function.Parameters["required"] = tool.Required
		}

		i.req.Tools = append(i.req.Tools, fn)
	}
}

func (i *interceptionBase) unmarshalArgs(in string) (args recorder.ToolArgs) {
	if len(strings.TrimSpace(in)) == 0 {
		return args // An empty string will fail JSON unmarshaling.
	}

	if err := json.Unmarshal([]byte(in), &args); err != nil {
		i.logger.Warn(context.Background(), "failed to unmarshal tool args", slog.Error(err))
	}

	return args
}

// writeUpstreamError marshals and writes a given error.
func (i *interceptionBase) writeUpstreamError(w http.ResponseWriter, oaiErr *intercept.ResponseError) {
	if oaiErr == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Set Retry-After when a cooldown is configured.
	if oaiErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(oaiErr.RetryAfter.Seconds()))))
	}
	w.WriteHeader(oaiErr.StatusCode)

	out, err := json.Marshal(oaiErr)
	if err != nil {
		i.logger.Warn(context.Background(), "failed to marshal upstream error", slog.Error(err), slog.F("error_payload", fmt.Sprintf("%+v", oaiErr)))
		// Response has to match expected format.
		_, _ = w.Write([]byte(`{
	"error": {
		"type": "error",
		"message":"error marshaling upstream error",
		"code": "server_error"
	}
}`))
	} else {
		_, _ = w.Write(out)
	}
}

// For centralized requests, markKeyOnError extracts an OpenAI
// SDK error from err and marks the key based on its status
// code. Returns true if the status was a key-specific failover
// trigger so callers can retry with the next key.
func (i *interceptionBase) markKeyOnError(ctx context.Context, key *keypool.Key, err error) bool {
	cp, ok := intercept.AsCentralizedPool(i.cred)
	if !ok {
		return false
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return cp.Pool.MarkKeyOnStatus(
		ctx, key, apiErr.Response, i.logger,
	)
}

func (i *interceptionBase) hasInjectableTools() bool {
	return i.mcpProxy != nil && len(i.mcpProxy.ListTools()) > 0
}

// recordTokenUsage records the token usage for a single completion, accounting
// for cached tokens included in the prompt token count.
func (i *interceptionBase) recordTokenUsage(ctx context.Context, msgID string, usage openai.CompletionUsage) {
	_ = i.recorder.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
		InterceptionID:       i.ID().String(),
		MsgID:                msgID,
		Input:                calculateActualInputTokenUsage(usage),
		Output:               usage.CompletionTokens,
		CacheReadInputTokens: usage.PromptTokensDetails.CachedTokens,
		ExtraTokenTypes: map[string]int64{
			"prompt_audio":                   usage.PromptTokensDetails.AudioTokens,
			"completion_accepted_prediction": usage.CompletionTokensDetails.AcceptedPredictionTokens,
			"completion_rejected_prediction": usage.CompletionTokensDetails.RejectedPredictionTokens,
			"completion_audio":               usage.CompletionTokensDetails.AudioTokens,
			"completion_reasoning":           usage.CompletionTokensDetails.ReasoningTokens,
		},
	})
}

func sumUsage(ref, in openai.CompletionUsage) openai.CompletionUsage {
	return openai.CompletionUsage{
		CompletionTokens: ref.CompletionTokens + in.CompletionTokens,
		PromptTokens:     ref.PromptTokens + in.PromptTokens,
		TotalTokens:      ref.TotalTokens + in.TotalTokens,
		CompletionTokensDetails: openai.CompletionUsageCompletionTokensDetails{
			AcceptedPredictionTokens: ref.CompletionTokensDetails.AcceptedPredictionTokens + in.CompletionTokensDetails.AcceptedPredictionTokens,
			AudioTokens:              ref.CompletionTokensDetails.AudioTokens + in.CompletionTokensDetails.AudioTokens,
			ReasoningTokens:          ref.CompletionTokensDetails.ReasoningTokens + in.CompletionTokensDetails.ReasoningTokens,
			RejectedPredictionTokens: ref.CompletionTokensDetails.RejectedPredictionTokens + in.CompletionTokensDetails.RejectedPredictionTokens,
		},
		PromptTokensDetails: openai.CompletionUsagePromptTokensDetails{
			AudioTokens:  ref.PromptTokensDetails.AudioTokens + in.PromptTokensDetails.AudioTokens,
			CachedTokens: ref.PromptTokensDetails.CachedTokens + in.PromptTokensDetails.CachedTokens,
		},
	}
}

// calculateActualInputTokenUsage accounts for cached tokens which are included in [openai.CompletionUsage].PromptTokens.
func calculateActualInputTokenUsage(in openai.CompletionUsage) int64 {
	// Input *includes* the cached tokens, so we subtract them here to reflect actual input token usage.
	// The original value can be reconstructed by adding CachedTokens back to Input.
	// See https://platform.openai.com/docs/api-reference/usage/completions_object#usage/completions_object-input_tokens.
	return max(0, in.PromptTokens /* The aggregated number of text input tokens used, including cached tokens. */ -
		in.PromptTokensDetails.CachedTokens /* The aggregated number of text input tokens that has been cached from previous requests. */)
}
