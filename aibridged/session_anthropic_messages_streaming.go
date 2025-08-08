package aibridged

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	ant_constant "github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/shared/constant"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var _ Session = &AnthropicMessagesStreamingSession{}

type AnthropicMessagesStreamingSession struct {
	AnthropicMessagesSessionBase
}

func NewAnthropicMessagesStreamingSession(req *BetaMessageNewParamsWrapper) *AnthropicMessagesStreamingSession {
	return &AnthropicMessagesStreamingSession{AnthropicMessagesSessionBase: AnthropicMessagesSessionBase{
		req: req,
	}}
}

func (s *AnthropicMessagesStreamingSession) Init(id string, logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) {
	s.AnthropicMessagesSessionBase.Init(id, logger.Named("streaming"), baseURL, key, tracker, toolMgr)
}

func (s *AnthropicMessagesStreamingSession) ProcessRequest(w http.ResponseWriter, r *http.Request) error {
	if s.req == nil {
		return xerrors.Errorf("developer error: req is nil")
	}

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	s.injectTools()

	// Track user prompt if not a small/fast model
	if !s.isSmallFastModel() {
		prompt, err := s.LastUserPrompt()
		if err != nil {
			s.logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
		} else if prompt != nil {
			if err := s.tracker.TrackPromptUsage(ctx, s.id, "", s.Model(), *prompt, nil); err != nil {
				s.logger.Warn(ctx, "failed to track prompt usage", slog.Error(err))
			}
		}
	}

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	es := newEventStream(anthropicEventStream)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := es.Close(streamCtx); err != nil {
				s.logger.Error(ctx, "error closing stream", slog.Error(err))
			}
		}()

		BasicSSESender(streamCtx, es, s.logger.Named("sse-sender")).ServeHTTP(w, r)
	}()

	// Add beta header if present in the request.
	var opts []option.RequestOption
	if reqBetaHeader := r.Header.Get("anthropic-beta"); strings.TrimSpace(reqBetaHeader) != "" {
		opts = append(opts, option.WithHeader("anthropic-beta", reqBetaHeader))
	}

	client := newAnthropicClient(s.baseURL, s.key, opts...)
	messages := s.req.BetaMessageNewParams
	logger := s.logger.With(slog.F("model", s.req.Model))

	isFirst := true
	for {
	newStream:
		stream := client.Beta.Messages.NewStreaming(streamCtx, messages)

		var message anthropic.BetaMessage
		var lastToolName string

		pendingToolCalls := make(map[string]string)

		for stream.Next() {
			event := stream.Current()
			if err := message.Accumulate(event); err != nil {
				logger.Error(ctx, "failed to accumulate streaming events", slog.Error(err), slog.F("event", event), slog.F("msg", message.RawJSON()))
				http.Error(w, "failed to proxy request", http.StatusInternalServerError)
				return xerrors.Errorf("failed to accumulate streaming events: %w", err)
			}

			// Tool-related handling.
			switch event.Type {
			case string(constant.ValueOf[ant_constant.ContentBlockStart]()):
				switch block := event.AsContentBlockStart().ContentBlock.AsAny().(type) {
				case anthropic.BetaToolUseBlock:
					lastToolName = block.Name

					if s.toolMgr.GetTool(block.Name) != nil {
						pendingToolCalls[block.Name] = block.ID
						// Don't relay this event back, otherwise the client will try invoke the tool as well.
						continue
					}
				}
			case string(constant.ValueOf[ant_constant.ContentBlockDelta]()):
				if len(pendingToolCalls) > 0 && s.toolMgr.GetTool(lastToolName) != nil {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(constant.ValueOf[ant_constant.ContentBlockStop]()):
				// Reset the tool name
				isInjected := s.toolMgr.GetTool(lastToolName) != nil
				lastToolName = ""

				if len(pendingToolCalls) > 0 && isInjected {
					// We're busy with a tool call, don't relay this event back.
					continue
				}
			case string(ant_constant.ValueOf[ant_constant.MessageStart]()):
				// Track token usage
				start := event.AsMessageStart()
				metadata := Metadata{
					"web_search_requests":      start.Message.Usage.ServerToolUse.WebSearchRequests,
					"cache_creation_input":     start.Message.Usage.CacheCreationInputTokens,
					"cache_read_input":         start.Message.Usage.CacheReadInputTokens,
					"cache_ephemeral_1h_input": start.Message.Usage.CacheCreation.Ephemeral1hInputTokens,
					"cache_ephemeral_5m_input": start.Message.Usage.CacheCreation.Ephemeral5mInputTokens,
				}
				if err := s.tracker.TrackTokensUsage(streamCtx, s.id, message.ID, s.Model(), start.Message.Usage.InputTokens, start.Message.Usage.OutputTokens, metadata); err != nil {
					logger.Warn(ctx, "failed to track token usage", slog.Error(err))
				}

				if !isFirst {
					// Don't send message_start unless first message!
					// We're sending multiple messages back and forth with the API, but from the client's perspective
					// they're just expecting a single message.
					continue
				}
			case string(ant_constant.ValueOf[ant_constant.MessageDelta]()):
				delta := event.AsMessageDelta()
				// Track token usage
				metadata := Metadata{
					"web_search_requests":  delta.Usage.ServerToolUse.WebSearchRequests,
					"cache_creation_input": delta.Usage.CacheCreationInputTokens,
					"cache_read_input":     delta.Usage.CacheReadInputTokens,
					// Note: CacheCreation fields are not available in MessageDeltaUsage
					"cache_ephemeral_1h_input": 0,
					"cache_ephemeral_5m_input": 0,
				}
				if err := s.tracker.TrackTokensUsage(streamCtx, s.id, message.ID, s.Model(), delta.Usage.InputTokens, delta.Usage.OutputTokens, metadata); err != nil {
					logger.Warn(ctx, "failed to track token usage", slog.Error(err))
				}

				// Don't relay message_delta events which indicate injected tool use.
				if len(pendingToolCalls) > 0 && s.toolMgr.GetTool(lastToolName) != nil {
					continue
				}

				// If currently calling a tool.
				if len(message.Content) > 0 && message.Content[len(message.Content)-1].Type == string(ant_constant.ValueOf[ant_constant.ToolUse]()) {
					toolName := message.Content[len(message.Content)-1].AsToolUse().Name
					if len(pendingToolCalls) > 0 && s.toolMgr.GetTool(toolName) != nil {
						continue
					}
				}

			// Don't send message_stop until all tools have been called.
			case string(ant_constant.ValueOf[ant_constant.MessageStop]()):

				if len(pendingToolCalls) > 0 {
					// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
					messages.Messages = append(messages.Messages, message.ToParam())

					for name, id := range pendingToolCalls {
						if s.toolMgr.GetTool(name) == nil {
							// Not an MCP proxy call, don't do anything.
							continue
						}

						tool := s.toolMgr.GetTool(name)
						if tool == nil {
							logger.Error(ctx, "tool not found in manager", slog.F("tool_name", name))
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
							logger.Error(ctx, "failed to find tool input", slog.F("tool_name", name), slog.F("found_tools", foundTools))
							continue
						}

						// Track injected tool usage - strip MCP tool namespacing if possible
						toolName := tool.Name
						if _, decodedTool, err := DecodeToolID(toolName); err == nil {
							toolName = decodedTool
						}
						if err := s.tracker.TrackToolUsage(streamCtx, s.id, message.ID, s.Model(), toolName, input, true, nil); err != nil {
							logger.Warn(ctx, "failed to track tool usage", slog.Error(err))
						}

						res, err := tool.Call(streamCtx, input)
						if err != nil {
							// Always provide a tool_result even if the tool call failed
							messages.Messages = append(messages.Messages,
								anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(id, fmt.Sprintf("Error calling tool: %v", err), true)),
							)
							continue
						}

						// Process tool result
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
									s.logger.Error(ctx, "unknown embedded resource type", slog.F("type", fmt.Sprintf("%T", resource)))
									toolResult.OfToolResult.Content = append(toolResult.OfToolResult.Content, anthropic.BetaToolResultBlockParamContentUnion{
										OfText: &anthropic.BetaTextBlockParam{
											Text: "Error: unknown embedded resource type",
										},
									})
									toolResult.OfToolResult.IsError = anthropic.Bool(true)
									hasValidResult = true
								}
							default:
								s.logger.Error(ctx, "not handling non-text tool result", slog.F("type", fmt.Sprintf("%T", cb)))
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
							s.logger.Error(ctx, "no tool result added", slog.F("content_len", len(res.Content)), slog.F("is_error", res.IsError))
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
							if s.toolMgr.GetTool(variant.Name) != nil {
								continue
							}

							if err := s.tracker.TrackToolUsage(streamCtx, s.id, message.ID, s.Model(), variant.Name, variant.Input, false, nil); err != nil {
								logger.Warn(ctx, "failed to track tool usage", slog.Error(err))
							}
						}
					}
				}
			}

			// Overwrite response identifier since proxy obscures injected tool call invocations.
			event.Message.ID = s.id
			if err := es.TrySend(streamCtx, event); err != nil {
				if isConnectionError(err) {
					s.logger.Debug(ctx, "client disconnected during sending event", slog.Error(err))
					return nil // Stop processing if client disconnected
				} else {
					s.logger.Error(ctx, "error during sending event", slog.Error(err))
				}
			}
		}

		var streamErr error
		if streamErr = stream.Err(); streamErr != nil {
			if isConnectionError(streamErr) {
				logger.Warn(ctx, "upstream connection closed", slog.Error(streamErr))
			} else {
				logger.Error(ctx, "anthropic stream error", slog.Error(streamErr))
				if antErr := getAnthropicErrorResponse(streamErr); antErr != nil {
					if err := es.TrySend(streamCtx, antErr); err != nil {
						logger.Error(ctx, "failed to send error", slog.Error(err))
					}
				}
			}
		}

		if err := es.Close(streamCtx); err != nil {
			logger.Error(ctx, "failed to close event stream", slog.Error(err))
		}

		wg.Wait()

		// Ensure we flush all the remaining data before ending.
		flush(w)

		if streamErr != nil {
			streamCancel(xerrors.Errorf("stream err: %w", streamErr))
		} else {
			streamCancel(xerrors.New("gracefully done"))
		}

		<-streamCtx.Done()
		break
	}

	return nil
}

func (s *AnthropicMessagesStreamingSession) Close() error {
	return nil
}
