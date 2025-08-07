package aibridged

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	clientFn func() (proto.DRPCAIBridgeDaemonClient, error)
	logger   slog.Logger

	tools map[string]*MCPTool
}

func handleOpenAI(provider *OpenAIChatProvider, drpcClient proto.DRPCAIBridgeDaemonClient, tools map[string][]*MCPTool, logger slog.Logger) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read and parse request.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			if isConnectionError(err) {
				logger.Debug(r.Context(), "client disconnected during request body read", slog.Error(err))
				return // Don't send error response if client already disconnected
			}
			logger.Error(r.Context(), "failed to read body", slog.Error(err))
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		req, err := provider.ParseRequest(body)
		if err != nil {
			logger.Error(r.Context(), "failed to parse request", slog.Error(err))
			http.Error(w, "failed to parse request", http.StatusBadRequest)
			return
		}

		// Create a new session.
		var sess Session
		if req.Stream {
			sess = provider.NewStreamingSession(req)
		} else {
			sess = provider.NewBlockingSession(req)
		}

		sessID := sess.Init(logger, provider.baseURL, provider.key, NewDRPCTracker(drpcClient), NewInjectedToolManager(tools))
		logger.Debug(context.Background(), "starting openai session", slog.F("session_id", sessID))

		defer func() {
			if err := sess.Close(); err != nil {
				logger.Warn(context.Background(), "failed to close session", slog.Error(err), slog.F("session_id", sessID), slog.F("kind", fmt.Sprintf("%T", sess)))
			}
		}()

		// Process the request.
		if err := sess.ProcessRequest(w, r); err != nil {
			logger.Error(r.Context(), "session execution failed", slog.Error(err))
		}
	}
}

func NewBridge(cfg codersdk.AIBridgeConfig, logger slog.Logger, clientFn func() (proto.DRPCAIBridgeDaemonClient, error), tools map[string][]*MCPTool) (*Bridge, error) {
	var bridge Bridge

	openAIProvider := NewOpenAIChatProvider(cfg.OpenAI.BaseURL.String(), cfg.OpenAI.Key.String())

	drpcClient, err := clientFn()
	if err != nil {
		return nil, xerrors.Errorf("could not acquire coderd client for tracking: %w", err)
	}

	mux := &http.ServeMux{}
	mux.HandleFunc("/v1/chat/completions", handleOpenAI(openAIProvider, drpcClient, tools, logger.Named("openai")))
	mux.HandleFunc("/v1/messages", bridge.proxyAnthropicRequest)

	srv := &http.Server{
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
	for _, serverTools := range tools {
		for _, tool := range serverTools {
			bridge.tools[tool.ID] = tool
		}
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

func (b *Bridge) Handler() http.Handler {
	return b.httpSrv.Handler
}

// TODO: track cumulative usage when tool invocations are executed; see OpenAI implementation.
func (b *Bridge) proxyAnthropicRequest(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.NewString()
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
	// It's highly unlikely that operators want to see these prompts tracked, but the token usage must be.
	// We could consider making this configurable in the future.
	isSmallFastModel := strings.Contains(string(in.Model), "3-5-haiku")

	// Find the most recent user message and track the prompt.
	if !isSmallFastModel {
		prompt, _ := in.LastUserPrompt() // TODO: error handling.
		if prompt != nil {
			b.trackUserPrompt(ctx, sessionID, "", string(in.Model), *prompt)
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
	// opts = append(opts, option.WithMiddleware(LoggingMiddleware))

	apiKey := b.cfg.Anthropic.Key.String()
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	opts = append(opts, option.WithAPIKey(apiKey))
	opts = append(opts, option.WithBaseURL(b.cfg.Anthropic.BaseURL.String()))

	// looks up API key with os.LookupEnv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient(opts...)
	if !in.UseStreaming() {
		opts = append(opts, option.WithRequestTimeout(time.Second*30)) // TODO: configurable.
		for {
			resp, err := client.Beta.Messages.New(ctx, messages, opts...)
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

				b.logger.Error(ctx, "upstream API error", slog.Error(err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			b.trackTokenUsage(ctx, sessionID, resp.ID, string(resp.Model), resp.Usage.InputTokens, resp.Usage.OutputTokens, map[string]int64{
				"web_search_requests":      resp.Usage.ServerToolUse.WebSearchRequests,
				"cache_creation_input":     resp.Usage.CacheCreationInputTokens,
				"cache_read_input":         resp.Usage.CacheReadInputTokens,
				"cache_ephemeral_1h_input": resp.Usage.CacheCreation.Ephemeral1hInputTokens,
				"cache_ephemeral_5m_input": resp.Usage.CacheCreation.Ephemeral5mInputTokens,
			})

			// Handle tool calls for non-streaming.
			var pendingToolCalls []anthropic.BetaToolUseBlock
			for _, c := range resp.Content {
				toolUse := c.AsToolUse()
				if toolUse.ID == "" {
					continue
				}

				if b.isInjectedTool(toolUse.Name) {
					pendingToolCalls = append(pendingToolCalls, toolUse)
					continue
				}

				// If tool is not injected, track it since the client will be handling it.
				b.trackToolUsage(ctx, sessionID, resp.ID, string(resp.Model), toolUse.Name, toolUse.Input, false)
			}

			// If no injected tool calls, we're done.
			if len(pendingToolCalls) == 0 {
				// Overwrite response identifier since proxy obscures injected tool call invocations.
				resp.ID = sessionID

				out, err := json.Marshal(resp)
				if err != nil {
					http.Error(w, "error marshaling response", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(out)
				break
			}

			// Append the assistant's message (which contains the tool_use block)
			// to the messages for the next API call.
			messages.Messages = append(messages.Messages, resp.ToParam())

			// Process each pending tool call.
			for _, tc := range pendingToolCalls {
				tool := b.tools[tc.Name]

				var args map[string]any
				serialized, err := json.Marshal(tc.Input)
				if err != nil {
					b.logger.Warn(ctx, "failed to marshal tool args for unmarshal", slog.Error(err), slog.F("tool", tc.Name))
					// Continue to next tool call, but still append an error tool_result
					messages.Messages = append(messages.Messages,
						anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error unmarshaling tool arguments: %v", err), true)),
					)
					continue
				} else if err := json.Unmarshal(serialized, &args); err != nil {
					b.logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err), slog.F("tool", tc.Name))
					// Continue to next tool call, but still append an error tool_result
					messages.Messages = append(messages.Messages,
						anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error unmarshaling tool arguments: %v", err), true)),
					)
					continue
				}

				b.trackToolUsage(ctx, sessionID, resp.ID, string(resp.Model), tc.Name, args, true)

				res, err := tool.Call(ctx, args)
				if err != nil {
					// Always provide a tool_result even if the tool call failed
					messages.Messages = append(messages.Messages,
						anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error calling tool: %v", err), true)),
					)
					continue
				}

				// Ensure at least one tool_result is always added for each tool_use.
				toolResult := anthropic.BetaContentBlockParamUnion{
					OfToolResult: &anthropic.BetaToolResultBlockParam{
						ToolUseID: tc.ID,
						IsError:   anthropic.Bool(false),
					},
				}

				var hasValidResult bool
				for _, content := range res.Content {
					switch cb := content.(type) {
					case mcp.TextContent:
						toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
							OfText: &anthropic.BetaTextBlockParam{
								Text: cb.Text,
							},
						})
						hasValidResult = true
					case mcp.EmbeddedResource:
						switch resource := cb.Resource.(type) {
						case mcp.TextResourceContents:
							val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
								resource.MIMEType, resource.URI, resource.Text)
							toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
								OfText: &anthropic.BetaTextBlockParam{
									Text: val,
								},
							})
							hasValidResult = true
						case mcp.BlobResourceContents:
							val := fmt.Sprintf("Binary resource (MIME: %s, URI: %s): %s",
								resource.MIMEType, resource.URI, resource.Blob)
							toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
								OfText: &anthropic.BetaTextBlockParam{
									Text: val,
								},
							})
							hasValidResult = true
						default:
							b.logger.Error(ctx, "unknown embedded resource type", slog.F("type", fmt.Sprintf("%T", resource)))
							toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
								OfText: &anthropic.BetaTextBlockParam{
									Text: "Error: unknown embedded resource type",
								},
							})
							toolResult.OfToolResult.IsError = anthropic.Bool(true)
							hasValidResult = true
						}
					default:
						b.logger.Error(ctx, "not handling non-text tool result", slog.F("type", fmt.Sprintf("%T", cb)))
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
					b.logger.Error(ctx, "no tool result added", slog.F("content_len", len(res.Content)), slog.F("is_error", res.IsError)) // This can only happen if there's somehow no content.
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
		}
		return
	}

	streamCtx, streamCancel := context.WithCancelCause(r.Context())

	es := newEventStream(anthropicEventStream)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := es.Close(streamCtx); err != nil {
				b.logger.Error(ctx, "error closing stream", slog.Error(err), slog.F("session_id", sessionID))
			}
		}()

		BasicSSESender(streamCtx, es, b.logger.Named("sse-sender")).ServeHTTP(w, r)
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
				return // TODO: don't return, skip to close.
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
				b.trackTokenUsage(streamCtx, sessionID, message.ID, string(message.Model), start.Message.Usage.InputTokens, start.Message.Usage.OutputTokens, map[string]int64{
					"web_search_requests":      start.Message.Usage.ServerToolUse.WebSearchRequests,
					"cache_creation_input":     start.Message.Usage.CacheCreationInputTokens,
					"cache_read_input":         start.Message.Usage.CacheReadInputTokens,
					"cache_ephemeral_1h_input": message.Usage.CacheCreation.Ephemeral1hInputTokens,
					"cache_ephemeral_5m_input": message.Usage.CacheCreation.Ephemeral5mInputTokens,
				})

				if !isFirst {
					// Don't send message_start unless first message!
					// We're sending multiple messages back and forth with the API, but from the client's perspective
					// they're just expecting a single message.
					continue
				}
			case string(ant_constant.ValueOf[ant_constant.MessageDelta]()):
				delta := event.AsMessageDelta()
				b.trackTokenUsage(streamCtx, sessionID, message.ID, string(message.Model), delta.Usage.InputTokens, delta.Usage.OutputTokens, map[string]int64{
					"web_search_requests":      delta.Usage.ServerToolUse.WebSearchRequests,
					"cache_creation_input":     delta.Usage.CacheCreationInputTokens,
					"cache_read_input":         delta.Usage.CacheReadInputTokens,
					"cache_ephemeral_1h_input": message.Usage.CacheCreation.Ephemeral1hInputTokens,
					"cache_ephemeral_5m_input": message.Usage.CacheCreation.Ephemeral5mInputTokens,
				})

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

						b.trackToolUsage(streamCtx, sessionID, message.ID, string(message.Model), tool.Name, input, true)

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

							b.trackToolUsage(streamCtx, sessionID, message.ID, string(message.Model), variant.Name, variant.Input, false)
						}
					}
				}
			}

			// Overwrite response identifier since proxy obscures injected tool call invocations.
			event.Message.ID = sessionID
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
		break
	}
}

func (b *Bridge) trackToolUsage(ctx context.Context, sessionID, msgID, model, toolName string, toolInput interface{}, injected bool) {
	coderdClient, err := b.clientFn()
	if err != nil {
		b.logger.Error(ctx, "could not acquire coderd client for tool usage tracking", slog.Error(err))
		return
	}

	var input string
	// TODO: unmarshal instead of marshal? i.e. persist maps rather than strings?
	switch val := toolInput.(type) {
	case string:
		input = val
	case []byte:
		input = string(val)
	default:
		encoded, err := json.Marshal(toolInput)
		if err == nil {
			input = string(encoded)
		} else {
			b.logger.Error(ctx, "failed to marshal tool input", slog.Error(err), slog.F("injected", injected), slog.F("tool_name", toolName))
			input = fmt.Sprintf("%v", val)
		}
	}

	// For injected tools: strip MCP tool namespacing, if possible.
	if injected {
		_, tool, err := DecodeToolID(toolName)
		// No recourse here; no point in logging - we'll see it in the database anyway.
		if err == nil {
			toolName = tool
		}
	}

	_, err = coderdClient.TrackToolUsage(ctx, &proto.TrackToolUsageRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Model:     model,
		Input:     input,
		Tool:      toolName,
		Injected:  injected,
	})
	if err != nil {
		b.logger.Error(ctx, "failed to track tool usage", slog.Error(err), slog.F("injected", injected))
	}
}

func (b *Bridge) trackUserPrompt(ctx context.Context, sessionID, msgID, model, prompt string) {
	coderdClient, err := b.clientFn()
	if err != nil {
		b.logger.Error(ctx, "could not acquire coderd client for user prompt tracking", slog.Error(err))
		return
	}

	_, err = coderdClient.TrackUserPrompt(ctx, &proto.TrackUserPromptRequest{
		SessionId: sessionID,
		MsgId:     msgID,
		Model:     model,
		Prompt:    prompt,
	})
	if err != nil {
		b.logger.Error(ctx, "failed to track user prompt", slog.Error(err))
	}
}

func (b *Bridge) trackTokenUsage(ctx context.Context, sessionID, msgID, model string, inputTokens, outputTokens int64, other map[string]int64) {
	coderdClient, err := b.clientFn()
	if err != nil {
		b.logger.Error(ctx, "could not acquire coderd client for token usage tracking", slog.Error(err))
		return
	}

	_, err = coderdClient.TrackTokenUsage(ctx, &proto.TrackTokenUsageRequest{
		SessionId:    sessionID,
		MsgId:        msgID,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Other:        other,
	})
	if err != nil {
		b.logger.Error(ctx, "failed to track token usage", slog.Error(err))
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

// logConnectionError logs connection errors with appropriate severity
func (b *Bridge) logConnectionError(ctx context.Context, err error, operation string) {
	if isConnectionError(err) {
		b.logger.Debug(ctx, "client disconnected during "+operation, slog.Error(err))
	} else {
		b.logger.Error(ctx, "error during "+operation, slog.Error(err))
	}
}
