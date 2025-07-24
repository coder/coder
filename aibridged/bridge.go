package aibridged

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	ant_constant "github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go"
	oai_option "github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared/constant"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

// Error type constants for structured error reporting
const (
	ErrorTypeRequestCanceled     = "request_canceled"
	ErrorTypeConnectionError     = "connection_error"
	ErrorTypeUnexpectedError     = "unexpected_error"
	ErrorTypeAnthropicAPIError   = "anthropic_api_error"
	ErrorTypeOpenAIAPIError      = "openai_api_error"
	ErrorTypeInternalError       = "internal_error"
	ErrorTypeValidationError     = "validation_error"
	ErrorTypeAuthenticationError = "authentication_error"
	ErrorTypeRateLimitError      = "rate_limit_error"
	ErrorTypeTimeoutError        = "timeout_error"
)

// BridgeError represents a structured error from the bridge that can carry
// specific error information back to the client.
type BridgeError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	StatusCode int               `json:"status_code"`
	Details    map[string]string `json:"details,omitempty"`
}

func (e *BridgeError) Error() string {
	return e.Message
}

// Bridge is responsible for proxying requests to upstream AI providers.
//
// Characteristics:
// 1.  Client-side cancel
// 2.  No timeout (SSE)
// 3a. client<->coderd conn established
// 3b. coderd<-> provider conn established
// 4a. requests from client<->coderd must be parsed, augmented, and relayed
// 4b. responses from provider->coderd must be parsed, optionally reflected back to client
// 5.  tool calls may be injected and intercepted, transparently to the client
// 6.  multiple calls can be made to provider while holding client<->coderd conn open
// 7.  client<->coderd conn must ONLY be closed on client-side disconn or coderd<->provider non-recoverable error.
type Bridge struct {
	cfg codersdk.AIBridgeConfig

	httpSrv  *http.Server
	addr     string
	clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)
	logger   slog.Logger

	tools map[string]*MCPTool
}

func NewBridge(cfg codersdk.AIBridgeConfig, addr string, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, bool), tools []*MCPTool) (*Bridge, error) {
	var bridge Bridge

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", bridge.proxyOpenAIRequest)
	mux.HandleFunc("/v1/messages", bridge.proxyAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,

		// TODO: configurable.
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // No write timeout for streaming responses.
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	bridge.cfg = cfg
	bridge.httpSrv = srv
	bridge.clientFn = clientFn
	bridge.logger = logger

	bridge.tools = make(map[string]*MCPTool, len(tools))
	for _, tool := range tools {
		bridge.tools[tool.ID] = tool
	}

	return &bridge, nil
}

func (b *Bridge) openAITarget() *url.URL {
	u := b.cfg.OpenAI.BaseURL.String()
	target, err := url.Parse(u)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q", u))
	}
	return target
}

// proxyOpenAIRequest intercepts, filters, augments, and delivers requests & responses from client to upstream and back.
//
// References:
//   - https://platform.openai.com/docs/api-reference/chat-streaming
func (b *Bridge) proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	b.logger.Info(r.Context(), "openai request started", slog.F("session_id", sessionID), slog.F("method", r.Method), slog.F("path", r.URL.Path))
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)

	defer func() {
		b.logger.Info(r.Context(), "openai request ended", slog.F("session_id", sessionID))
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if isConnectionError(err) {
			b.logger.Debug(r.Context(), "client disconnected during request body read", slog.Error(err))
			return // Don't send error response if client already disconnected
		}
		b.logger.Error(r.Context(), "failed to read body", slog.Error(err))
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	var in ChatCompletionNewParamsWrapper
	if err = json.Unmarshal(body, &in); err != nil {
		b.logger.Error(r.Context(), "failed to unmarshal request", slog.Error(err))
		http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
		return
	}

	prompt, err := in.LastUserPrompt() // TODO: error handling.
	if prompt != nil {
		if _, err = coderdClient.TrackUserPrompt(ctx, &proto.TrackUserPromptRequest{
			Model:  in.Model,
			Prompt: *prompt,
		}); err != nil {
			b.logger.Error(r.Context(), "failed to track user prompt", slog.Error(err))
		}
	}

	// Prepend assistant message.
	in.Messages = append([]openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant that explicitly mentions when tool calls are about to be made."),
	}, in.Messages...)

	for _, tool := range b.tools {
		fn := openai.ChatCompletionToolParam{
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
		}

		// Otherwise the request fails with "None is not of type 'array'" if a nil slice is given.
		if len(tool.Required) > 0 {
			// Must list ALL properties when strict=true.
			fn.Function.Parameters["required"] = tool.Required
		}

		in.Tools = append(in.Tools, fn)
	}

	// Configure OpenAI client with authentication
	var opts []oai_option.RequestOption
	apiKey := b.cfg.OpenAI.Key.String()
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	opts = append(opts, oai_option.WithAPIKey(apiKey))
	if baseURL := b.cfg.OpenAI.BaseURL.String(); baseURL != "" {
		opts = append(opts, oai_option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(opts...)
	req := in.ChatCompletionNewParams

	if in.Stream {
		in.StreamOptions.IncludeUsage = openai.Bool(true)

		streamCtx, streamCancel := context.WithCancelCause(ctx)
		defer streamCancel(xerrors.New("deferred"))

		events := newEventStream(openAIEventStream)

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() {
				if err := events.Close(streamCtx); err != nil {
					b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("session_id", sessionID))
				}
			}()

			BasicSSESender(streamCtx, sessionID, "", events, b.logger.Named("sse-sender")).ServeHTTP(w, r)
		}()

		// TODO: implement parallel tool calls.
		// TODO: don't send if not supported by model (i.e. o4-mini).
		req.ParallelToolCalls = openai.Bool(false)

		var (
			stream          *ssestream.Stream[openai.ChatCompletionChunk]
			cumulativeUsage openai.CompletionUsage
		)
		for {
			var pendingToolCalls []openai.FinishedChatCompletionToolCall

			stream = client.Chat.Completions.NewStreaming(ctx, req)
			var acc openai.ChatCompletionAccumulator
			for stream.Next() {
				chunk := stream.Current()
				acc.AddChunk(chunk)

				fmt.Printf("[in]: %s\n", chunk.RawJSON())

				shouldRelayChunk := true
				if toolCall, ok := acc.JustFinishedToolCall(); ok {
					// Don't intercept and handle builtin tools.
					if b.isInjectedTool(toolCall.Name) {
						pendingToolCalls = append(pendingToolCalls, toolCall)
						// Don't relay this chunk back; we'll handle it transparently.
						shouldRelayChunk = false
					} else {
						_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
							Model: string(in.Model),
							Input: toolCall.Arguments,
							Tool:  toolCall.Name,
						})
						if err != nil {
							b.logger.Error(ctx, "failed to track tool usage", slog.Error(err))
						}
					}
				}

				if len(pendingToolCalls) > 0 {
					// Any chunks following a tool call invocation should not be relayed.
					shouldRelayChunk = false
				}

				cumulativeUsage = sumUsage(cumulativeUsage, chunk.Usage)

				if shouldRelayChunk {
					// If usage information is available, relay the cumulative usage once all tool invocations have completed.
					if chunk.Usage.CompletionTokens > 0 {
						chunk.Usage = cumulativeUsage
					}

					events.TrySend(ctx, chunk)

					fmt.Printf("\t[out]: %s\n", chunk.RawJSON())
				}
			}

			// If the usage information is set, track it.
			// The API will send usage information when the response terminates, which will happen if a tool call is invoked.
			if _, err = coderdClient.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
				MsgId:        acc.ID,
				Model:        string(acc.Model),
				InputTokens:  cumulativeUsage.PromptTokens,
				OutputTokens: cumulativeUsage.CompletionTokens,
				Other: map[string]int64{
					"prompt_audio":                   cumulativeUsage.PromptTokensDetails.AudioTokens,
					"prompt_cached":                  cumulativeUsage.PromptTokensDetails.CachedTokens,
					"completion_accepted_prediction": cumulativeUsage.CompletionTokensDetails.AcceptedPredictionTokens,
					"completion_rejected_prediction": cumulativeUsage.CompletionTokensDetails.RejectedPredictionTokens,
					"completion_audio":               cumulativeUsage.CompletionTokensDetails.AudioTokens,
					"completion_reasoning":           cumulativeUsage.CompletionTokensDetails.ReasoningTokens,
				},
			}); err != nil {
				b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
			}

			if err := stream.Err(); err != nil {
				b.logger.Error(ctx, "server stream error", slog.Error(err))
				var apierr *openai.Error
				if errors.As(err, &apierr) {
					events.TrySend(ctx, map[string]interface{}{
						"error":   true,
						"message": err.Error(),
					})
					// http.Error(w, apierr.Message, apierr.StatusCode)
					break
				} else if isConnectionError(err) {
					b.logger.Warn(ctx, "upstream connection error", slog.Error(err))
				}

				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			if len(pendingToolCalls) == 0 {
				break
			}

			appendedPrevMsg := false
			for _, tc := range pendingToolCalls {
				if !b.isInjectedTool(tc.Name) {
					// Not an MCP proxy call, don't do anything.
					continue
				}

				// Only do this once.
				if !appendedPrevMsg {
					// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
					req.Messages = append(req.Messages, acc.Choices[len(acc.Choices)-1].Message.ToParam())
					appendedPrevMsg = true
				}

				_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
					Model:    string(in.Model),
					Input:    tc.Arguments,
					Tool:     tc.Name,
					Injected: true,
				})
				if err != nil {
					b.logger.Error(ctx, "failed to track injected tool usage", slog.Error(err))
				}

				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					b.logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err), slog.F("tool", tc.Name))
				}

				res, err := b.tools[tc.Name].Call(streamCtx, args)
				if err != nil {
					// Always provide a tool_result even if the tool call failed
					errorResponse := map[string]interface{}{
						"error":   true,
						"message": err.Error(),
					}
					errorJSON, _ := json.Marshal(errorResponse)
					req.Messages = append(req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
					continue
				}

				var out strings.Builder
				if err := json.NewEncoder(&out).Encode(res); err != nil {
					b.logger.Error(ctx, "failed to encode tool response", slog.Error(err))
					// Always provide a tool_result even if encoding failed
					// TODO: abstract.
					errorResponse := map[string]interface{}{
						"error":   true,
						"message": err.Error(),
					}
					errorJSON, _ := json.Marshal(errorResponse)
					req.Messages = append(req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
					continue
				}

				req.Messages = append(req.Messages, openai.ToolMessage(out.String(), tc.ID))
			}
		}

		err = events.Close(streamCtx)
		if err != nil {
			b.logger.Error(ctx, "failed to close event stream", slog.Error(err))
		}

		wg.Wait()

		// Ensure we flush all the remaining data before ending.
		flush(w)

		if err != nil {
			streamCancel(xerrors.Errorf("stream err: %w", err))
		} else {
			streamCancel(xerrors.New("gracefully done"))
		}

		<-streamCtx.Done()
	} else {
		// Non-streaming case with tool calling support
		var cumulativeUsage openai.CompletionUsage

		for {
			completion, err := client.Chat.Completions.New(ctx, req)
			if err != nil {
				if isConnectionError(err) {
					b.logger.Debug(ctx, "upstream connection closed", slog.Error(err))
					return // Don't send error response if client already disconnected
				}
				var apierr *openai.Error
				if errors.As(err, &apierr) {
					http.Error(w, apierr.Message, apierr.StatusCode)
					return
				}

				b.logger.Error(ctx, "chat completion failed", slog.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Track cumulative usage
			cumulativeUsage = sumUsage(cumulativeUsage, completion.Usage)

			// Track token usage
			if _, err = coderdClient.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
				MsgId:        completion.ID,
				Model:        string(completion.Model),
				InputTokens:  cumulativeUsage.PromptTokens,
				OutputTokens: cumulativeUsage.CompletionTokens,
				Other: map[string]int64{
					"prompt_audio":                   cumulativeUsage.PromptTokensDetails.AudioTokens,
					"prompt_cached":                  cumulativeUsage.PromptTokensDetails.CachedTokens,
					"completion_accepted_prediction": cumulativeUsage.CompletionTokensDetails.AcceptedPredictionTokens,
					"completion_rejected_prediction": cumulativeUsage.CompletionTokensDetails.RejectedPredictionTokens,
					"completion_audio":               cumulativeUsage.CompletionTokensDetails.AudioTokens,
					"completion_reasoning":           cumulativeUsage.CompletionTokensDetails.ReasoningTokens,
				},
			}); err != nil {
				b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
			}

			// Check if we have tool calls to process
			var pendingToolCalls []openai.ChatCompletionMessageToolCall
			if len(completion.Choices) > 0 && completion.Choices[0].Message.ToolCalls != nil {
				for _, toolCall := range completion.Choices[0].Message.ToolCalls {
					if b.isInjectedTool(toolCall.Function.Name) {
						pendingToolCalls = append(pendingToolCalls, toolCall)
					} else {
						_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
							Model: string(in.Model),
							Input: toolCall.Function.Arguments,
							Tool:  toolCall.Function.Name,
						})
						if err != nil {
							b.logger.Error(ctx, "failed to track tool usage", slog.Error(err))
						}
					}
				}
			}

			// If no injected tool calls, we're done
			if len(pendingToolCalls) == 0 {
				// Update the cumulative usage in the final response
				if completion.Usage.CompletionTokens > 0 {
					completion.Usage = cumulativeUsage
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(completion.RawJSON()))
				break
			}

			appendedPrevMsg := false
			// Process each pending tool call
			for _, tc := range pendingToolCalls {
				if !b.isInjectedTool(tc.ID) {
					// Not an MCP proxy call, don't do anything.
					continue
				}
				// Only do this once.
				if !appendedPrevMsg {
					// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
					req.Messages = append(req.Messages, completion.Choices[0].Message.ToParam())
					appendedPrevMsg = true
				}

				_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
					Model:    string(in.Model),
					Input:    tc.Function.Arguments,
					Tool:     tc.Function.Name,
					Injected: true,
				})
				if err != nil {
					b.logger.Error(ctx, "failed to track injected tool usage", slog.Error(err))
				}

				var (
					args map[string]string
					buf  bytes.Buffer
				)
				_ = json.NewEncoder(&buf).Encode(tc.Function.Arguments)
				_ = json.NewDecoder(&buf).Decode(&args)
				res, err := b.tools[tc.ID].Call(ctx, args)
				if err != nil {
					// Always provide a tool result even if the tool call failed
					errorResponse := map[string]interface{}{
						"error":   true,
						"message": err.Error(),
					}
					errorJSON, _ := json.Marshal(errorResponse)
					req.Messages = append(req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
					continue
				}

				var out strings.Builder
				if err := json.NewEncoder(&out).Encode(res); err != nil {
					b.logger.Error(ctx, "failed to encode tool response", slog.Error(err))
					// Always provide a tool result even if encoding failed
					errorResponse := map[string]interface{}{
						"error":   true,
						"message": err.Error(),
					}
					errorJSON, _ := json.Marshal(errorResponse)
					req.Messages = append(req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
					continue
				}

				req.Messages = append(req.Messages, openai.ToolMessage(out.String(), tc.ID))
			}
		}
	}
}

func LoggingMiddleware(req *http.Request, next option.MiddlewareNext) (res *http.Response, err error) {
	reqOut, _ := httputil.DumpRequest(req, true)

	// Forward the request to the next handler
	res, err = next(req)

	isSmallFastModel := strings.Contains(string(reqOut), "3-5-haiku")
	if isSmallFastModel {
		return res, err
	}

	fmt.Printf("[req] %s\n", reqOut)

	// Handle stuff after the request
	if err != nil {
		return res, err
	}

	respOut, _ := httputil.DumpResponse(res, true)
	fmt.Printf("[resp] %s\n", respOut)

	return res, err
}

// TODO: track cumulative usage when tool invocations are executed; see OpenAI implementation.
func (b *Bridge) proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	b.logger.Info(r.Context(), "anthropic request started", slog.F("session_id", sessionID), slog.F("method", r.Method), slog.F("path", r.URL.Path))
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)

	defer func() {
		b.logger.Info(r.Context(), "anthropic request ended", slog.F("session_id", sessionID))
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	coderdClient, ok := b.clientFn()
	if !ok {
		// TODO: log issue.
		http.Error(w, "could not acquire coderd client", http.StatusInternalServerError)
		return
	}

	useBeta := r.URL.Query().Get("beta") == "true"
	if !useBeta {
		b.logger.Warn(r.Context(), "non-beta API requested, using beta instead", slog.F("url", r.URL.String()))
		useBeta = true
		// http.Error(w, "only beta API supported", http.StatusInternalServerError)
		// return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		b.logger.Error(r.Context(), "failed to read body", slog.Error(err))
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	// var in streamer
	// if useBeta {
	var in BetaMessageNewParamsWrapper
	// } else {
	//	in = &MessageNewParamsWrapper{}
	//}

	if err = json.Unmarshal(body, &in); err != nil {
		b.logger.Error(r.Context(), "failed to unmarshal request", slog.Error(err))
		http.Error(w, "failed to unmarshal request", http.StatusInternalServerError)
		return
	}

	// Policy examples.
	if strings.Contains(string(in.Model), "opus") {
		err := xerrors.Errorf("%q model is not allowed", in.Model)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, m := range in.Messages {
		for _, c := range m.Content {
			if c.OfText == nil {
				continue
			}

			if strings.Contains(c.OfText.Text, ".env") {
				http.Error(w, "Request blocked due to attempted access to sensitive file; this has been logged.", http.StatusBadRequest)
				return
			}
		}
	}

	for _, t := range in.Tools {
		if t.OfTool == nil {
			continue
		}

		if strings.Contains(t.OfTool.Name, "mcp__") && (!strings.Contains(t.OfTool.Name, "go") && !strings.Contains(t.OfTool.Name, "typescript")) {
			segs := strings.Split(t.OfTool.Name, "__")
			var serverName string
			if len(segs) >= 1 {
				serverName = segs[1]
			}

			http.Error(w, fmt.Sprintf("Request blocked due to MCP server %q being used; this has been logged.", serverName), http.StatusBadRequest)
			return
		}
	}

	for _, tool := range b.tools {
		in.Tools = append(in.Tools, anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				InputSchema: anthropic.BetaToolInputSchemaParam{
					Properties: tool.Params,
					Required:   tool.Required,
				},
				Name:        tool.ID,
				Description: anthropic.String(tool.Description),
				Type:        anthropic.BetaToolTypeCustom,
			},
		})
	}

	// Claude Code uses the 3.5 Haiku model to do autocomplete and other small tasks. (see ANTHROPIC_SMALL_FAST_MODEL).
	isSmallFastModel := strings.Contains(string(in.Model), "3-5-haiku")

	// Find the most recent user message and track the prompt.
	if !isSmallFastModel {
		prompt, err := in.LastUserPrompt() // TODO: error handling.
		if prompt != nil {
			if _, err = coderdClient.TrackUserPrompt(ctx, &proto.TrackUserPromptRequest{
				Prompt: *prompt,
				Model:  string(in.Model),
			}); err != nil {
				b.logger.Error(r.Context(), "failed to track user prompt", slog.Error(err))
			}
		}
	}

	messages := in.BetaMessageNewParams

	// Note: Parallel tool calls are disabled in the processing loop to avoid tool_use/tool_result block mismatches
	messages.ToolChoice = anthropic.BetaToolChoiceUnionParam{
		OfAny: &anthropic.BetaToolChoiceAnyParam{
			Type:                   "auto",
			DisableParallelToolUse: anthropic.Bool(true),
		},
	}

	var opts []option.RequestOption
	if reqBetaHeader := r.Header.Get("anthropic-beta"); strings.TrimSpace(reqBetaHeader) != "" {
		opts = append(opts, option.WithHeader("anthropic-beta", reqBetaHeader))
	}
	opts = append(opts, option.WithMiddleware(LoggingMiddleware))

	apiKey := b.cfg.Anthropic.Key.String()
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	opts = append(opts, option.WithAPIKey(apiKey))
	opts = append(opts, option.WithBaseURL(b.cfg.Anthropic.BaseURL.String()))

	// looks up API key with os.LookupEnv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient(opts...)
	if !in.UseStreaming() {
		var resp *anthropic.BetaMessage
		for {
			resp, err = client.Beta.Messages.New(ctx, messages, opts...)
			if err != nil {
				if isConnectionError(err) {
					b.logger.Warn(ctx, "upstream connection closed", slog.Error(err))
					return
				}

				b.logger.Error(ctx, "anthropic stream error", slog.Error(err))
				if antErr := getAnthropicErrorResponse(err); antErr != nil {
					http.Error(w, antErr.Error.Message, antErr.StatusCode)
					return
				}
			}

			if _, err = coderdClient.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
				MsgId:        resp.ID,
				Model:        string(resp.Model),
				InputTokens:  resp.Usage.InputTokens,
				OutputTokens: resp.Usage.OutputTokens,
				Other: map[string]int64{
					"web_search_requests":      resp.Usage.ServerToolUse.WebSearchRequests,
					"cache_creation_input":     resp.Usage.CacheCreationInputTokens,
					"cache_read_input":         resp.Usage.CacheReadInputTokens,
					"cache_ephemeral_1h_input": resp.Usage.CacheCreation.Ephemeral1hInputTokens,
					"cache_ephemeral_5m_input": resp.Usage.CacheCreation.Ephemeral5mInputTokens,
				},
			}); err != nil {
				b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
			}

			messages.Messages = append(messages.Messages, resp.ToParam())

			if resp.StopReason == anthropic.BetaStopReasonEndTurn {
				break
			}

			if resp.StopReason == anthropic.BetaStopReasonToolUse {
				var (
					toolUse anthropic.BetaToolUseBlock
					input   any
				)
				for _, c := range resp.Content {
					toolUse = c.AsToolUse()
					if toolUse.ID == "" {
						continue
					}

					input = toolUse.Input
				}

				if input != nil {
					if serialized, err := json.Marshal(input); err == nil {
						_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
							Model:    string(resp.Model),
							Input:    string(serialized),
							Tool:     toolUse.Name,
							Injected: b.isInjectedTool(toolUse.Name),
						})
						if err != nil {
							b.logger.Error(ctx, "failed to track injected tool usage", slog.Error(err))
						}
					} else {
						b.logger.Warn(ctx, "failed to marshal args for tool usage", slog.Error(err))
					}
				}

				break
			}
		}

		out, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "error marshaling response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return
	}

	streamCtx, streamCancel := context.WithCancelCause(r.Context())

	es := newEventStream(anthropicEventStream)

	// var buf strings.Builder
	// in.Messages[0].Content = []anthropic.BetaContentBlockParamUnion{in.Messages[0].Content[len(in.Messages[0].Content) - 1]}
	//
	// json.NewEncoder(&buf).Encode(in)
	// fmt.Println(strings.Replace(buf.String(), "'", "\\'", -1))

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := es.Close(streamCtx); err != nil {
				b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("session_id", sessionID))
			}
		}()

		BasicSSESender(streamCtx, sessionID, string(in.Model), es, b.logger.Named("sse-sender")).ServeHTTP(w, r)
	}()

	isFirst := true
	for {
	newStream:
		stream := client.Beta.Messages.NewStreaming(streamCtx, messages)

		var events []anthropic.BetaRawMessageStreamEventUnion
		var message anthropic.BetaMessage
		var lastToolName string

		pendingToolCalls := make(map[string]string)

		for stream.Next() {
			event := stream.Current()
			events = append(events, event)

			if err := message.Accumulate(event); err != nil {
				b.logger.Error(ctx, "failed to accumulate streaming events", slog.Error(err), slog.F("event", event), slog.F("msg", message.RawJSON()))
				http.Error(w, "failed to proxy request", http.StatusInternalServerError)
				return
			}

			// Tool-related handling.
			switch event.Type {
			case string(constant.ValueOf[ant_constant.ContentBlockStart]()): // Have to do this because otherwise content_block_delta and content_block_start both match the type anthropic.BetaRawContentBlockStartEvent
				switch block := event.AsContentBlockStart().ContentBlock.AsAny().(type) {
				case anthropic.BetaToolUseBlock:
					lastToolName = block.Name

					if b.isInjectedTool(block.Name) {
						pendingToolCalls[block.Name] = block.ID
						// Don't relay this event back, otherwise the client will try invoke the tool as well.
						continue
					}
				default:
					fmt.Printf("[%s] %s\n", event.Type, event.RawJSON())
				}
			case string(constant.ValueOf[ant_constant.ContentBlockDelta]()):
				if len(pendingToolCalls) > 0 && b.isInjectedTool(lastToolName) {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(constant.ValueOf[ant_constant.ContentBlockStop]()):
				// Reset the tool name
				isInjected := b.isInjectedTool(lastToolName)
				lastToolName = ""

				if len(pendingToolCalls) > 0 && isInjected {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(ant_constant.ValueOf[ant_constant.MessageStart]()):
				// Anthropic's docs only mention usage in message_delta events, but it's also present in message_start.
				// See https://docs.anthropic.com/en/docs/build-with-claude/streaming#event-types.
				start := event.AsMessageStart()
				if _, err = coderdClient.TrackTokenUsage(streamCtx, &proto.TrackTokenUsageRequest{
					MsgId:        message.ID,
					Model:        string(message.Model),
					InputTokens:  start.Message.Usage.InputTokens,
					OutputTokens: start.Message.Usage.OutputTokens,
					Other: map[string]int64{
						"web_search_requests":      start.Message.Usage.ServerToolUse.WebSearchRequests,
						"cache_creation_input":     start.Message.Usage.CacheCreationInputTokens,
						"cache_read_input":         start.Message.Usage.CacheReadInputTokens,
						"cache_ephemeral_1h_input": message.Usage.CacheCreation.Ephemeral1hInputTokens,
						"cache_ephemeral_5m_input": message.Usage.CacheCreation.Ephemeral5mInputTokens,
					},
				}); err != nil {
					b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
				}

				if !isFirst {
					// Don't send message_start unless first message!
					// We're sending multiple messages back and forth with the API, but from the client's perspective
					// they're just expecting a single message.
					continue
				}
			case string(ant_constant.ValueOf[ant_constant.MessageDelta]()):
				delta := event.AsMessageDelta()
				if _, err = coderdClient.TrackTokenUsage(streamCtx, &proto.TrackTokenUsageRequest{
					MsgId:        message.ID,
					Model:        string(message.Model),
					InputTokens:  delta.Usage.InputTokens,
					OutputTokens: delta.Usage.OutputTokens,
					Other: map[string]int64{
						"web_search_requests":      delta.Usage.ServerToolUse.WebSearchRequests,
						"cache_creation_input":     delta.Usage.CacheCreationInputTokens,
						"cache_read_input":         delta.Usage.CacheReadInputTokens,
						"cache_ephemeral_1h_input": message.Usage.CacheCreation.Ephemeral1hInputTokens,
						"cache_ephemeral_5m_input": message.Usage.CacheCreation.Ephemeral5mInputTokens,
					},
				}); err != nil {
					b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
				}

				// Don't relay message_delta events which indicate injected tool use.
				if len(pendingToolCalls) > 0 && b.isInjectedTool(lastToolName) {
					continue
				}

				// If currently calling a tool.
				if message.Content[len(message.Content)-1].Type == string(ant_constant.ValueOf[ant_constant.ToolUse]()) {
					toolName := message.Content[len(message.Content)-1].AsToolUse().Name
					if len(pendingToolCalls) > 0 && b.isInjectedTool(toolName) {
						continue
					}
				}

			// Don't send message_stop until all tools have been called.
			case string(ant_constant.ValueOf[ant_constant.MessageStop]()):

				if len(pendingToolCalls) > 0 {
					// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
					messages.Messages = append(messages.Messages, message.ToParam())

					for name, id := range pendingToolCalls {
						if !b.isInjectedTool(name) {
							// Not an MCP proxy call, don't do anything.
							continue
						}

						tool := b.tools[name]

						var (
							input      any
							foundTool  bool
							foundTools int
						)
						for _, block := range message.Content {
							switch variant := block.AsAny().(type) {
							case anthropic.BetaToolUseBlock:
								foundTools++
								if variant.Name == name {
									input = variant.Input
									foundTool = true
								}
							}
						}

						if !foundTool {
							b.logger.Error(ctx, "failed to find tool input", slog.F("tool_name", name), slog.F("found_tools", foundTools))
							continue
						}

						fmt.Printf("[event] %s\n[tool(%q)] %s %+v\n\n", event.RawJSON(), id, name, input)

						if serialized, err := json.Marshal(input); err == nil {
							_, err = coderdClient.TrackToolUsage(streamCtx, &proto.TrackToolUsageRequest{
								Model:    string(message.Model),
								Input:    string(serialized),
								Tool:     tool.Name,
								Injected: true,
							})
							if err != nil {
								b.logger.Error(ctx, "failed to track injected tool usage", slog.Error(err))
							}
						} else {
							b.logger.Warn(ctx, "failed to marshal args for tool usage", slog.Error(err))
						}

						res, err := b.tools[tool.ID].Call(streamCtx, input)
						if err != nil {
							// Always provide a tool_result even if the tool call failed
							messages.Messages = append(messages.Messages,
								anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, fmt.Sprintf("Error calling tool: %v", err), true)),
							)
							continue
						}

						var out strings.Builder
						if err := json.NewEncoder(&out).Encode(res); err != nil {
							b.logger.Error(ctx, "failed to encode tool response", slog.Error(err))
							// Always provide a tool_result even if encoding failed
							messages.Messages = append(messages.Messages,
								anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, fmt.Sprintf("Error encoding tool response: %v", err), true)),
							)
							continue
						}

						// Ensure at least one tool_result is always added for each tool_use
						toolResult := anthropic.BetaContentBlockParamUnion{
							OfToolResult: &anthropic.BetaToolResultBlockParam{
								ToolUseID: id,
								IsError:   anthropic.Bool(false),
							},
						}

						var hasValidResult bool
						for _, content := range res.Content {
							switch cb := content.(type) {
							case mcp.TextContent:
								// messages.Messages = append(messages.Messages,
								//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, cb.Text, false)),
								//)
								toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
									OfText: &anthropic.BetaTextBlockParam{
										Text: cb.Text,
									},
								})

								hasValidResult = true
							case mcp.EmbeddedResource:
								// Handle embedded resource based on its type
								switch resource := cb.Resource.(type) {
								case mcp.TextResourceContents:
									// For text resources, include the text content
									val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
										resource.MIMEType, resource.URI, resource.Text)
									// messages.Messages = append(messages.Messages,
									//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, val, false)),
									//)

									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
										OfText: &anthropic.BetaTextBlockParam{
											Text: val,
										},
									})
									hasValidResult = true
								case mcp.BlobResourceContents:
									// For blob resources, include the base64 data with MIME type info
									val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
										resource.MIMEType, resource.URI, resource.Blob)
									// messages.Messages = append(messages.Messages,
									//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, val, false)),
									//)

									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
										OfText: &anthropic.BetaTextBlockParam{
											Text: val,
										},
									})
									hasValidResult = true
								default:
									b.logger.Error(ctx, "unknown embedded resource type", slog.F("type", fmt.Sprintf("%T", resource)))
									// messages.Messages = append(messages.Messages,
									//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, "Error: unknown embedded resource type", true)),
									//)

									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
										OfText: &anthropic.BetaTextBlockParam{
											Text: "Error: unknown embedded resource type",
										},
									})
									toolResult.OfToolResult.IsError = anthropic.Bool(true)
									hasValidResult = true
								}
							default:
								// Not supported - but we must still provide a tool_result to match the tool_use
								b.logger.Error(ctx, "not handling non-text tool result", slog.F("type", fmt.Sprintf("%T", cb)), slog.F("json", out.String()))
								// messages.Messages = append(messages.Messages,
								//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, "Error: unsupported tool result type", true)),
								//)

								toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
									OfText: &anthropic.BetaTextBlockParam{
										Text: "Error: unsupported tool result type",
									},
								})
								toolResult.OfToolResult.IsError = anthropic.Bool(true)
								hasValidResult = true
							}
						}

						// If no content was processed, still add a tool_result
						if !hasValidResult {
							// messages.Messages = append(messages.Messages,
							//	anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, "Error: no valid tool result content", true)),
							//)

							toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
								OfText: &anthropic.BetaTextBlockParam{
									Text: "Error: no valid tool result content",
								},
							})
							toolResult.OfToolResult.IsError = anthropic.Bool(true)
						}

						if len(toolResult.OfToolResult.Content) > 0 {
							messages.Messages = append(messages.Messages, anthropic.NewBetaUserMessage(toolResult))
						}
					}

					// Causes a new stream to be run with updated messages.
					isFirst = false
					goto newStream
				} else {
					// Find all the non-injected tools and track their uses.
					for _, block := range message.Content {
						switch variant := block.AsAny().(type) {
						case anthropic.BetaToolUseBlock:
							if b.isInjectedTool(variant.Name) {
								continue
							}

							if serialized, err := json.Marshal(variant.Input); err == nil {
								_, err = coderdClient.TrackToolUsage(streamCtx, &proto.TrackToolUsageRequest{
									Model: string(message.Model),
									Input: string(serialized),
									Tool:  variant.Name,
								})
								if err != nil {
									b.logger.Error(ctx, "failed to track non-injected tool usage", slog.Error(err))
								}
							} else {
								b.logger.Warn(ctx, "failed to marshal args for tool usage", slog.Error(err))
							}
						}
					}
				}
			}

			if err := es.TrySend(streamCtx, event); err != nil {
				b.logConnectionError(ctx, err, "sending event")
				if isConnectionError(err) {
					return // Stop processing if client disconnected
				}
			}
		}

		var streamErr error
		if streamErr = stream.Err(); streamErr != nil {
			if isConnectionError(streamErr) {
				b.logger.Warn(ctx, "upstream connection closed", slog.Error(streamErr))
			}

			b.logger.Error(ctx, "anthropic stream error", slog.Error(streamErr))
			if antErr := getAnthropicErrorResponse(streamErr); antErr != nil {
				err = es.TrySend(streamCtx, antErr)
				if err != nil {
					b.logger.Error(ctx, "failed to send error", slog.Error(err))
				}
			}
		}

		err = es.Close(streamCtx)
		if err != nil {
			b.logger.Error(ctx, "failed to close event stream", slog.Error(err))
		}

		wg.Wait()

		// Ensure we flush all the remaining data before ending.
		flush(w)

		if err != nil || streamErr != nil {
			streamCancel(xerrors.Errorf("stream err: %w", err))
		} else {
			streamCancel(xerrors.New("gracefully done"))
		}

		<-streamCtx.Done()

		// TODO: do we need to do this?
		// // Close the underlying connection by hijacking it
		// if hijacker, ok := w.(http.Hijacker); ok {
		// 	conn, _, err := hijacker.Hijack()
		// 	if err != nil {
		// 		b.logger.Error(ctx, "failed to hijack connection", slog.Error(err))
		// 	} else {
		// 		conn.Close() // This closes the TCP connection entirely
		// 		b.logger.Debug(ctx, "connection closed, stream over")
		// 	}
		// }

		break
	}
}

func (b *Bridge) isInjectedTool(id string) bool {
	_, ok := b.tools[id]
	return ok
}

func getAnthropicErrorResponse(err error) *AnthropicErrorResponse {
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		return nil
	}

	msg := apierr.Error()

	var detail *anthropic.BetaAPIError
	if field, ok := apierr.JSON.ExtraFields["error"]; ok {
		_ = json.Unmarshal([]byte(field.Raw()), &detail)
	}
	if detail != nil {
		msg = detail.Message
	}

	return &AnthropicErrorResponse{
		BetaErrorResponse: &anthropic.BetaErrorResponse{
			Error: anthropic.BetaErrorUnion{
				Message: msg,
				Type:    string(detail.Type),
			},
			Type: ant_constant.ValueOf[ant_constant.Error](),
		},
		StatusCode: apierr.StatusCode,
	}
}

type AnthropicErrorResponse struct {
	*anthropic.BetaErrorResponse

	StatusCode int `json:"-"`
}

func (b *Bridge) Serve() error {
	list, err := net.Listen("tcp", b.httpSrv.Addr)
	if err != nil {
		return xerrors.Errorf("listen: %w", err)
	}

	b.addr = list.Addr().String()

	return b.httpSrv.Serve(list) // TODO: TLS.
}

func (b *Bridge) Addr() string {
	return b.addr
}

// logConnectionError logs connection errors with appropriate severity
func (b *Bridge) logConnectionError(ctx context.Context, err error, operation string) {
	if isConnectionError(err) {
		b.logger.Debug(ctx, "client disconnected during "+operation, slog.Error(err))
	} else {
		b.logger.Error(ctx, "error during "+operation, slog.Error(err))
	}
}
