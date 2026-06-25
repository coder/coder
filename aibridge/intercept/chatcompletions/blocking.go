package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/intercept/eventstream"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
)

type BlockingInterception struct {
	interceptionBase
}

func NewBlockingInterceptor(
	id uuid.UUID,
	req *ChatCompletionNewParamsWrapper,
	cfg intercept.Config,
	cred intercept.Credential,
	clientHeaders http.Header,
	tracer trace.Tracer,
) *BlockingInterception {
	return &BlockingInterception{interceptionBase: interceptionBase{
		id:            id,
		req:           req,
		cfg:           cfg,
		cred:          cred,
		clientHeaders: clientHeaders,
		tracer:        tracer,
	}}
}

func (i *BlockingInterception) Setup(logger slog.Logger, rec recorder.Recorder, mcpProxy mcp.ServerProxier) {
	i.interceptionBase.Setup(logger.Named("blocking"), rec, mcpProxy)
}

func (*BlockingInterception) Streaming() bool {
	return false
}

func (i *BlockingInterception) TraceAttributes(r *http.Request) []attribute.KeyValue {
	return i.interceptionBase.baseTraceAttributes(r, false)
}

func (i *BlockingInterception) ProcessRequest(w http.ResponseWriter, r *http.Request) (outErr error) {
	if i.req == nil {
		return xerrors.New("developer error: req is nil")
	}

	ctx, span := i.tracer.Start(r.Context(), "Intercept.ProcessRequest", trace.WithAttributes(tracing.InterceptionAttributesFromContext(r.Context())...))
	defer tracing.EndSpanErr(span, &outErr)

	svc := i.newCompletionsService(ctx)
	logger := i.logger.With(slog.F("model", i.req.Model))

	var (
		cumulativeUsage openai.CompletionUsage
		completion      *openai.ChatCompletion
		err             error
	)

	i.injectTools()

	prompt, err := i.req.lastUserPrompt()
	if err != nil {
		logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
	}

	// Sum the key attempts across all iterations and record once when the
	// interception completes.
	var totalKeyAttempts int
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		defer func() {
			cp.Pool.RecordAttempts(totalKeyAttempts)
		}()
	}

	for {
		// TODO add outer loop span (https://github.com/coder/aibridge/issues/67)

		var opts []option.RequestOption
		opts = append(opts, option.WithRequestTimeout(time.Second*600))

		// TODO(ssncferreira): inject actor headers directly in the client-header
		//   middleware instead of using SDK options.
		if actor := aibcontext.ActorFromContext(r.Context()); actor != nil && i.cfg.SendActorHeaders {
			opts = append(opts, intercept.ActorHeadersAsOpenAIOpts(actor)...)
		}

		var keyAttempts int
		completion, keyAttempts, err = i.newChatCompletion(ctx, svc, opts)
		totalKeyAttempts += keyAttempts
		if err != nil {
			break
		}

		if prompt != nil {
			_ = i.recorder.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
				InterceptionID: i.ID().String(),
				MsgID:          completion.ID,
				Prompt:         *prompt,
			})
			prompt = nil
		}

		lastUsage := completion.Usage
		cumulativeUsage = sumUsage(cumulativeUsage, completion.Usage)

		i.recordTokenUsage(ctx, completion.ID, lastUsage)

		// Check if we have tool calls to process.
		var pendingToolCalls []openai.ChatCompletionMessageToolCallUnion
		if len(completion.Choices) > 0 && completion.Choices[0].Message.ToolCalls != nil {
			for _, toolCall := range completion.Choices[0].Message.ToolCalls {
				if i.mcpProxy != nil && i.mcpProxy.GetTool(toolCall.Function.Name) != nil {
					pendingToolCalls = append(pendingToolCalls, toolCall)
				} else {
					_ = i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
						InterceptionID: i.ID().String(),
						MsgID:          completion.ID,
						ToolCallID:     toolCall.ID,
						Tool:           toolCall.Function.Name,
						Args:           i.unmarshalArgs(toolCall.Function.Arguments),
						Injected:       false,
					})
				}
			}
		}

		// If no injected tool calls, we're done.
		if len(pendingToolCalls) == 0 {
			break
		}

		appendedPrevMsg := false
		for _, tc := range pendingToolCalls {
			if i.mcpProxy == nil {
				continue
			}

			tool := i.mcpProxy.GetTool(tc.Function.Name)
			if tool == nil {
				// Not a known tool, don't do anything.
				logger.Warn(ctx, "pending tool call for non-managed tool, skipping", slog.F("tool", tc.Function.Name))
				continue
			}
			// Only do this once.
			if !appendedPrevMsg {
				// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
				i.req.Messages = append(i.req.Messages, completion.Choices[0].Message.ToParam())
				appendedPrevMsg = true
			}

			args := i.unmarshalArgs(tc.Function.Arguments)
			res, err := tool.Call(ctx, args, i.tracer)
			_ = i.recorder.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
				InterceptionID:  i.ID().String(),
				MsgID:           completion.ID,
				ToolCallID:      tc.ID,
				ServerURL:       &tool.ServerURL,
				Tool:            tool.Name,
				Args:            args,
				Injected:        true,
				InvocationError: err,
			})

			if err != nil {
				// Always provide a tool result even if the tool call failed
				errorResponse := map[string]interface{}{
					// TODO: interception ID?
					"error":   true,
					"message": err.Error(),
				}
				errorJSON, _ := json.Marshal(errorResponse)
				i.req.Messages = append(i.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			var out strings.Builder
			if err := json.NewEncoder(&out).Encode(res); err != nil {
				logger.Warn(ctx, "failed to encode tool response", slog.Error(err))
				// Always provide a tool result even if encoding failed
				errorResponse := map[string]interface{}{
					// TODO: interception ID?
					"error":   true,
					"message": err.Error(),
				}
				errorJSON, _ := json.Marshal(errorResponse)
				i.req.Messages = append(i.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			i.req.Messages = append(i.req.Messages, openai.ToolMessage(out.String(), tc.ID))
		}
	}

	if err != nil {
		if eventstream.IsConnError(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return xerrors.Errorf("upstream connection closed: %w", err)
		}

		// The failover loop may return a keypool exhaustion
		// error. Check before the SDK-error path.
		var keyPoolErr *keypool.Error
		if errors.As(err, &keyPoolErr) {
			i.writeUpstreamError(w, intercept.ResponseErrorFromKeyPool(keyPoolErr))
			return xerrors.Errorf("key pool exhausted: %w", err)
		}

		if apiErr := intercept.ResponseErrorFromAPIError(err); apiErr != nil {
			i.writeUpstreamError(w, apiErr)
			return xerrors.Errorf("openai API error: %w", err)
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return xerrors.Errorf("chat completion failed: %w", err)
	}

	if completion == nil {
		return nil
	}

	// Overwrite response identifier since proxy obscures injected tool call invocations.
	completion.ID = i.ID().String()

	// Update the cumulative usage in the final response.
	if completion.Usage.CompletionTokens > 0 {
		completion.Usage = cumulativeUsage
	}

	w.Header().Set("Content-Type", "application/json")
	out, err := json.Marshal(completion)
	if err != nil {
		out, _ = json.Marshal(i.newErrorResponse(xerrors.Errorf("failed to marshal response: %w", err)))
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	_, _ = w.Write(out)

	return nil
}

// newChatCompletion routes by credential type, returning the upstream
// completion, the number of key attempts made for this call, and any error. A
// centralized key pool fails over across keys, while BYOK authenticates with a
// single, fixed credential baked into svc, so it makes one attempt.
func (i *BlockingInterception) newChatCompletion(ctx context.Context, svc openai.ChatCompletionService, opts []option.RequestOption) (*openai.ChatCompletion, int, error) {
	if cp, ok := intercept.AsCentralizedPool(i.cred); ok {
		return i.newChatCompletionWithKeyFailover(ctx, svc, cp, opts)
	}
	completion, err := i.newChatCompletionWithKey(intercept.WithCredentialInfo(ctx, i.cred), svc, opts)
	return completion, 0, err
}

// newChatCompletionWithKey performs a single upstream call.
func (i *BlockingInterception) newChatCompletionWithKey(ctx context.Context, svc openai.ChatCompletionService, opts []option.RequestOption) (_ *openai.ChatCompletion, outErr error) {
	_, span := i.tracer.Start(ctx, "Intercept.ProcessRequest.Upstream", trace.WithAttributes(tracing.InterceptionAttributesFromContext(ctx)...))
	defer tracing.EndSpanErr(span, &outErr)

	requestOpts, overrideBody, err := i.chatCompletionRequestOptions(opts)
	if err != nil {
		return nil, xerrors.Errorf("prepare request body: %w", err)
	}
	params := i.req.ChatCompletionNewParams
	if overrideBody {
		params = openai.ChatCompletionNewParams{}
	}
	return svc.New(ctx, params, requestOpts...)
}

// newChatCompletionWithKeyFailover walks the centralized key pool, trying each
// key until one succeeds or the pool is exhausted. Keys are marked temporary
// on 429 and permanent on 401/403. Errors that aren't key-specific don't
// trigger failover and are returned to the caller. It returns the upstream
// completion, the number of key attempts made for this call, and any error.
func (i *BlockingInterception) newChatCompletionWithKeyFailover(ctx context.Context, svc openai.ChatCompletionService, cp *intercept.CentralizedPool, opts []option.RequestOption) (*openai.ChatCompletion, int, error) {
	walker := cp.Pool.Walker()
	for {
		key, keyPoolErr := cp.NextKey(walker)
		if keyPoolErr != nil {
			return nil, walker.Attempts(), keyPoolErr
		}

		ctx = intercept.WithCredentialInfo(ctx, i.cred)
		i.logger.Debug(ctx, "using centralized api key")
		requestOpts := append([]option.RequestOption{}, opts...)
		requestOpts = append(requestOpts,
			option.WithAPIKey(key.Value()),
			// Disable SDK retries because the failover loop
			// handles retries via key rotation.
			option.WithMaxRetries(0),
		)
		completion, err := i.newChatCompletionWithKey(ctx, svc, requestOpts)
		// Key-specific failure: try the next key.
		if i.markKeyOnError(ctx, key, err) {
			continue
		}
		// Either success (completion, nil) or a non-key error
		// (nil, err): nothing to retry, return as-is.
		return completion, walker.Attempts(), err
	}
}
