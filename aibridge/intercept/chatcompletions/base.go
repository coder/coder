package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/aibridge/config"
	aibcontext "github.com/coder/aibridge/context"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/intercept/apidump"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/recorder"
	"github.com/coder/aibridge/tracing"
	"github.com/coder/quartz"

	"cdr.dev/slog/v3"
)

type interceptionBase struct {
	id           uuid.UUID
	providerName string
	req          *ChatCompletionNewParamsWrapper
	cfg          config.OpenAI

	// clientHeaders are the original HTTP headers from the client request.
	clientHeaders  http.Header
	authHeaderName string

	logger slog.Logger
	tracer trace.Tracer

	recorder   recorder.Recorder
	mcpProxy   mcp.ServerProxier
	credential intercept.CredentialInfo
}

func (i *interceptionBase) newCompletionsService() openai.ChatCompletionService {
	opts := []option.RequestOption{option.WithAPIKey(i.cfg.Key), option.WithBaseURL(i.cfg.BaseURL)}
	if i.cfg.MaxRetries != nil {
		opts = append(opts, option.WithMaxRetries(*i.cfg.MaxRetries))
	}

	// Add extra headers if configured.
	// Some providers require additional headers that are not added by the SDK.
	// TODO(ssncferreira): remove as part of https://github.com/coder/aibridge/issues/192
	for key, value := range i.cfg.ExtraHeaders {
		opts = append(opts, option.WithHeader(key, value))
	}

	// Forward client headers to upstream. This middleware runs after the SDK
	// has built the request, and replaces the outgoing headers with the sanitized
	// client headers plus provider auth.
	if i.clientHeaders != nil {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			req.Header = intercept.BuildUpstreamHeaders(req.Header, i.clientHeaders, i.authHeaderName)
			return next(req)
		}))
	}

	// Add API dump middleware if configured
	if mw := apidump.NewBridgeMiddleware(i.cfg.APIDumpDir, i.providerName, i.Model(), i.id, i.logger, quartz.NewReal()); mw != nil {
		opts = append(opts, option.WithMiddleware(mw))
	}

	return openai.NewChatCompletionService(opts...)
}

func (i *interceptionBase) ID() uuid.UUID {
	return i.id
}

func (i *interceptionBase) Credential() intercept.CredentialInfo {
	return i.credential
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
		attribute.String(tracing.Provider, i.providerName),
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
func (i *interceptionBase) writeUpstreamError(w http.ResponseWriter, oaiErr *responseError) {
	if oaiErr == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
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
	},
}`))
	} else {
		_, _ = w.Write(out)
	}
}

func (i *interceptionBase) hasInjectableTools() bool {
	return i.mcpProxy != nil && len(i.mcpProxy.ListTools()) > 0
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
	return in.PromptTokens /* The aggregated number of text input tokens used, including cached tokens. */ -
		in.PromptTokensDetails.CachedTokens /* The aggregated number of text input tokens that has been cached from previous requests. */
}

func getErrorResponse(err error) *responseError {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return nil
	}

	return &responseError{
		ErrorObject: &shared.ErrorObject{
			Code:    apiErr.Code,
			Message: apiErr.Message,
			Type:    apiErr.Type,
		},
		StatusCode: apiErr.StatusCode,
	}
}

var _ error = &responseError{}

type responseError struct {
	ErrorObject *shared.ErrorObject `json:"error"`
	StatusCode  int                 `json:"-"`
}

func newErrorResponse(msg error) *responseError {
	return &responseError{
		ErrorObject: &shared.ErrorObject{
			Code:    "error",
			Message: msg.Error(),
			Type:    "error",
		},
	}
}

func (a *responseError) Error() string {
	if a.ErrorObject == nil {
		return ""
	}
	return a.ErrorObject.Message
}
