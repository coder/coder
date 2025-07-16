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

type Bridge struct {
	cfg codersdk.AIBridgeConfig

	httpSrv  *http.Server
	addr     string
	clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)
	logger   slog.Logger

	lastErr    error
	mcpBridges map[string]*MCPToolBridge
}

func NewBridge(cfg codersdk.AIBridgeConfig, addr string, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, bool)) (*Bridge, error) {
	var bridge Bridge

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", bridge.proxyOpenAIRequest)
	mux.HandleFunc("/v1/messages", bridge.proxyAnthropicRequest)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
		// TODO: other settings.
	}

	bridge.cfg = cfg
	bridge.httpSrv = srv
	bridge.clientFn = clientFn
	bridge.logger = logger

	// const (
	// 	githubMCPName = "github"
	// 	coderMCPName  = "coder"
	// )
	// githubMCP, err := NewMCPToolBridge(githubMCPName, "https://api.githubcopilot.com/mcp/", map[string]string{
	// 	"Authorization": "Bearer " + os.Getenv("GITHUB_MCP_TOKEN"),
	// }, logger.Named("mcp-bridge-github"))
	// if err != nil {
	// 	return nil, xerrors.Errorf("github MCP bridge setup: %w", err)
	// }
	// coderMCP, err := NewMCPToolBridge(coderMCPName, "https://dev.coder.com/api/experimental/mcp/http", map[string]string{
	// 	"Authorization": "Bearer " + os.Getenv("CODER_MCP_TOKEN"),
	// 	// This is necessary to even access the MCP endpoint.
	// 	"Coder-Session-Token": os.Getenv("CODER_MCP_SESSION_TOKEN"),
	// }, logger.Named("mcp-bridge-coder"))
	// if err != nil {
	// 	return nil, xerrors.Errorf("coder MCP bridge setup: %w", err)
	// }

	// bridge.mcpBridges = map[string]*MCPToolBridge{
	// 	githubMCPName: githubMCP,
	// 	coderMCPName:  coderMCP,
	// }

	// ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	// defer cancel()

	// var eg errgroup.Group
	// eg.Go(func() error {
	// 	err := githubMCP.Init(ctx)
	// 	if err == nil {
	// 		return nil
	// 	}
	// 	return xerrors.Errorf("github: %w", err)
	// })
	// eg.Go(func() error {
	// 	err := coderMCP.Init(ctx)
	// 	if err == nil {
	// 		return nil
	// 	}
	// 	return xerrors.Errorf("coder: %w", err)
	// })

	// // This must block requests until MCP proxies are setup.
	// if err := eg.Wait(); err != nil {
	// 	return nil, xerrors.Errorf("MCP proxy init: %w", err)
	// }

	return &bridge, nil
}

func (b *Bridge) openAITarget() *url.URL {
	u := b.cfg.OpenAIBaseURL.String()
	target, err := url.Parse(u)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q", u))
	}
	return target
}

func (b *Bridge) proxyOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	b.logger.Info(r.Context(), "OpenAI request started", slog.F("sessionID", sessionID), slog.F("method", r.Method), slog.F("path", r.URL.Path))
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)

	// Clear any previous error state
	b.clearError()

	defer func() {
		b.logger.Info(r.Context(), "OpenAI request ended", slog.F("sessionID", sessionID))
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	// Required characteristics:
	// 1.  Client-side cancel
	// 2.  No timeout (SSE)
	// 3a. client->coderd conn established
	// 3b. coderd->AI provider conn established
	// 4.  responses from AI provider->coderd must be parsed, optionally reflected back to client
	// 5.  tool calls must be injected and intercepted, transparently to the client
	// 6.  multiple calls can be made to AI provider while holding client->coderd conn open
	// 7.  client->coderd conn must ONLY be closed on client-side disconn or coderd->AI provider non-recoverable error.

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

	in.Tools = []openai.ChatCompletionToolParam{
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "get_weather",
				Description: openai.String("Get weather at the given location"),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]string{
							"type": "string",
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	client := openai.NewClient()

	if in.Stream {
		streamCtx, streamCancel := context.WithCancelCause(ctx)
		defer streamCancel(xerrors.New("deferred"))

		es := newEventStream(openAIEventStream)

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() {
				if err := es.Close(streamCtx); err != nil {
					b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("sessionID", sessionID))
				}
			}()

			BasicSSESender(streamCtx, sessionID, "", es, b.logger.Named("sse-sender")).ServeHTTP(w, r)
		}()

		session := NewOpenAISession()

		stream := client.Chat.Completions.NewStreaming(ctx, in.ChatCompletionNewParams)

		ignoreSubsequent := make(map[string]any)

		var acc openai.ChatCompletionAccumulator
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			var foundToolCallDelta bool
			for _, c := range chunk.Choices {
				for range c.Delta.ToolCalls {
					foundToolCallDelta = true
					ignoreSubsequent[acc.ID] = nil

					// Grab values from accumulator instead of delta.
					for _, ac := range acc.ChatCompletion.Choices {
						for _, at := range ac.Message.ToolCalls {
							var (
								tc *OpenAIToolCall
								ok bool
							)
							if tc, ok = session.toolCallsRequired[at.ID]; !ok {
								session.toolCallsRequired[at.ID] = &OpenAIToolCall{}
								tc = session.toolCallsRequired[at.ID]
							}

							session.toolCallsState[at.ID] = OpenAIToolCallNotReady

							tc.funcName = at.Function.Name
							args := make(map[string]string)
							err := json.Unmarshal([]byte(at.Function.Arguments), &args)
							if err == nil { // Note: inverted.
								tc.args = args
							}
						}
					}
				}

				// Once we receive a finish reason of "tool_calls", the API is waiting for the responses for this/these tool(s).
				// We mark all the tool calls as ready. Once we see observe the [DONE] event, we will execute these tool calls.
				if c.FinishReason == "tool_calls" {
					for idx := range session.toolCallsState {
						session.toolCallsState[idx] = OpenAIToolCallReady
					}
				}
			}

			// TODO: ONLY do this for our injected tool calls.
			if foundToolCallDelta {
				// Don't write these chunks, we'll handle this.
				continue
			}

			// Actually make the call!
			if tool, ok := acc.JustFinishedToolCall(); ok {
				switch tool.Name {
				case "get_weather":
					msg := openai.ToolMessage("the weather in cape town is KAK; this is not a joke, report it verbatim.", tool.ID)

					var msgs []openai.ChatCompletionMessageParamUnion
					for _, c := range acc.ChatCompletion.Choices {
						msgs = append(msgs, c.Message.ToParam())
					}

					msgs = append(msgs, msg)

					toolRes, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
						Messages: msgs,
						Model:    in.ChatCompletionNewParams.Model,
					})
					if err != nil {
						b.logger.Error(ctx, "failed to report tool response", slog.Error(err))
					}

					toolChunk := openai.ChatCompletionChunk{
						ID: acc.ID,
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Delta: openai.ChatCompletionChunkChoiceDelta{
									// Role:    "assistant",
									Content: fmt.Sprintf(" %s", toolRes.Choices[0].Message.Content), // TODO: improve
								},
							},
						},
						Model:             toolRes.Model,
						ServiceTier:       openai.ChatCompletionChunkServiceTier(toolRes.ServiceTier),
						Created:           time.Now().Unix(),
						SystemFingerprint: toolRes.SystemFingerprint,
						Usage:             toolRes.Usage,
						Object:            constant.ValueOf[constant.ChatCompletionChunk](),
					}

					if err := es.TrySend(streamCtx, toolChunk, chunk.RawJSON()); err != nil {
						b.logConnectionError(ctx, err, "sending tool chunk")
						if isConnectionError(err) {
							return // Stop processing if client disconnected
						}
					}

					finishChunk := openai.ChatCompletionChunk{
						ID: acc.ID,
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Delta: openai.ChatCompletionChunkChoiceDelta{
									// Role:    "assistant",
									Content: "",
								},
								FinishReason: string(openai.CompletionChoiceFinishReasonStop),
							},
						},
						Model:             toolRes.Model,
						ServiceTier:       openai.ChatCompletionChunkServiceTier(toolRes.ServiceTier),
						Created:           time.Now().Unix(),
						SystemFingerprint: toolRes.SystemFingerprint,
						Usage:             toolRes.Usage,
						Object:            constant.ValueOf[constant.ChatCompletionChunk](),
					}

					if err := es.TrySend(streamCtx, finishChunk, chunk.RawJSON(), "choices[].delta.content"); err != nil {
						b.logConnectionError(ctx, err, "sending finish chunk")
						if isConnectionError(err) {
							return // Stop processing if client disconnected
						}
					}
				}
				continue
			}

			// TODO: clean this up. Once we receive a tool invocation we need to hijack the conversation, since the client
			// 		 won't be handling the tool call if auto-injected. That means that any subsequent events which wrap
			//		 up the stream need to be ignored because we send those after the tool call is executed and the result
			//		 is appended as if it came from the assistant.
			if _, ok := ignoreSubsequent[acc.ID]; !ok {
				if err := es.TrySend(streamCtx, chunk, chunk.RawJSON()); err != nil {
					b.logConnectionError(ctx, err, "sending reflected chunk")
					if isConnectionError(err) {
						return // Stop processing if client disconnected
					}
				}
			}
		}

		if err := es.Close(streamCtx); err != nil {
			b.logger.Error(ctx, "failed to close event stream", slog.Error(err))
		}

		if err := stream.Err(); err != nil {
			if isConnectionError(err) {
				b.logger.Debug(ctx, "upstream connection closed", slog.Error(err))
			} else {
				b.logger.Error(ctx, "server stream error", slog.Error(err))
				b.setError(err)
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		wg.Wait()

		// Ensure we flush all the remaining data before ending.
		flush(w)

		streamCancel(xerrors.New("gracefully done"))

		<-streamCtx.Done()
	} else {
		completion, err := client.Chat.Completions.New(ctx, in.ChatCompletionNewParams)
		if err != nil {
			b.logger.Error(ctx, "chat completion failed", slog.Error(err))
			b.setError(err)
			http.Error(w, "chat completion failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK) // TODO: always?
		_, _ = w.Write([]byte(completion.RawJSON()))
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

func (b *Bridge) proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.New()
	b.logger.Info(r.Context(), "Anthropic request started", slog.F("sessionID", sessionID), slog.F("method", r.Method), slog.F("path", r.URL.Path))
	_, _ = fmt.Fprintf(os.Stderr, "[%s] new chat session started\n\n", sessionID)

	// Clear any previous error state
	b.clearError()

	defer func() {
		b.logger.Info(r.Context(), "Anthropic request ended", slog.F("sessionID", sessionID))
		_, _ = fmt.Fprintf(os.Stderr, "[%s] chat session ended\n\n", sessionID)
	}()

	// out, _ := httputil.DumpRequest(r, true)
	// fmt.Printf("\n\nREQUEST: %s\n\n", out)

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
		b.setError(err)
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

	for _, proxy := range b.mcpBridges {
		in.Tools = append(in.Tools, proxy.ListTools()...)
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

	// Lib will automatically look for ANTHROPIC_API_KEY env.
	if _, ok := os.LookupEnv("ANTHROPIC_API_KEY"); !ok {
		opts = append(opts, option.WithAPIKey(b.cfg.Anthropic.Key.String()))
	}
	opts = append(opts, option.WithBaseURL(b.cfg.Anthropic.BaseURL.String()))

	// looks up API key with os.LookupEnv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient(opts...)
	if !in.UseStreaming() {
		http.Error(w, "streaming API supported only", http.StatusBadRequest)
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
				b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("sessionID", sessionID))
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

			// TODO: this should *ignore* built-in tools; so ONLY do this for injected tooling.

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
						serverName, toolName, found := parseToolName(name)
						if !found {
							// Not an MCP proxy call, don't do anything.
							continue
						}

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

						var (
							serialized map[string]string
							buf        bytes.Buffer
						)
						_ = json.NewEncoder(&buf).Encode(input)
						_ = json.NewDecoder(&buf).Decode(&serialized)

						fmt.Printf("[event] %s\n[tool(%q)] %s %+v\n\n", event.RawJSON(), id, name, input)

						_, err = coderdClient.TrackToolUsage(streamCtx, &proto.TrackToolUsageRequest{
							Model:    string(message.Model),
							Input:    serialized,
							Tool:     toolName,
							Injected: true,
						})
						if err != nil {
							b.logger.Error(ctx, "failed to track injected tool usage", slog.Error(err))
						}

						res, err := b.mcpBridges[serverName].CallTool(streamCtx, toolName, input)
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

							var (
								serialized map[string]string
								buf        bytes.Buffer
							)
							_ = json.NewEncoder(&buf).Encode(variant.Input)
							_ = json.NewDecoder(&buf).Decode(&serialized)
							_, err = coderdClient.TrackToolUsage(streamCtx, &proto.TrackToolUsageRequest{
								Model: string(message.Model),
								Input: serialized,
								Tool:  variant.Name,
							})
							if err != nil {
								b.logger.Error(ctx, "failed to track non-injected tool usage", slog.Error(err))
							}
						}
					}
				}
			}

			if err := es.TrySend(streamCtx, event, event.RawJSON()); err != nil {
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
				err = es.TrySend(streamCtx, antErr, "<error>")
				if err != nil {
					b.logger.Error(ctx, "failed to send error", slog.Error(err))
				}

				b.setError(antErr)
			} else {
				b.setError(streamErr)
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

func parseToolName(name string) (string, string, bool) {
	serverName, toolName, found := strings.Cut(name, MCPProxyDelimiter)
	return serverName, toolName, found
}

func (b *Bridge) isInjectedTool(name string) bool {
	serverName, toolName, found := parseToolName(name)
	if !found {
		return false
	}

	mcp, ok := b.mcpBridges[serverName]
	if !ok {
		return false
	}

	return mcp.HasTool(toolName)
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

// setError sets a structured error with appropriate context
func (b *Bridge) setError(val any) {
	switch err := val.(type) {
	case error:
		switch {
		case errors.Is(err, context.Canceled):
			b.lastErr = &BridgeError{
				Code:       ErrorTypeRequestCanceled,
				Message:    "Request was canceled",
				StatusCode: http.StatusRequestTimeout,
			}
		case isConnectionError(err):
			b.lastErr = &BridgeError{
				Code:       ErrorTypeConnectionError,
				Message:    "Connection to upstream service failed",
				StatusCode: http.StatusBadGateway,
			}
		default:
			b.lastErr = &BridgeError{
				Code:       ErrorTypeUnexpectedError,
				Message:    err.Error(),
				StatusCode: http.StatusInternalServerError,
			}
		}
	case AnthropicErrorResponse:
		b.lastErr = &BridgeError{
			Code:       ErrorTypeAnthropicAPIError,
			Message:    err.Error.Message,
			Details:    map[string]string{"type": err.Error.Type},
			StatusCode: err.StatusCode,
		}
	}
}

// clearError clears the error state when a new request starts
func (b *Bridge) clearError() {
	b.lastErr = nil
}

// logConnectionError logs connection errors with appropriate severity
func (b *Bridge) logConnectionError(ctx context.Context, err error, operation string) {
	if isConnectionError(err) {
		b.logger.Debug(ctx, "client disconnected during "+operation, slog.Error(err))
	} else {
		b.logger.Error(ctx, "error during "+operation, slog.Error(err))
	}
}
