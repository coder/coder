package aibridged

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var _ Session = &AnthropicMessagesBlockingSession{}

type AnthropicMessagesBlockingSession struct {
	AnthropicMessagesSessionBase
}

func NewAnthropicMessagesBlockingSession(req *BetaMessageNewParamsWrapper, baseURL, key string) *AnthropicMessagesBlockingSession {
	return &AnthropicMessagesBlockingSession{AnthropicMessagesSessionBase: AnthropicMessagesSessionBase{
		req:     req,
		baseURL: baseURL,
		key:     key,
	}}
}

func (s *AnthropicMessagesBlockingSession) Init(logger slog.Logger, tracker Tracker, toolMgr ToolManager) string {
	return s.AnthropicMessagesSessionBase.Init(logger.Named("blocking"), tracker, toolMgr)
}

func (s *AnthropicMessagesBlockingSession) ProcessRequest(w http.ResponseWriter, r *http.Request) error {
	if s.req == nil {
		return xerrors.Errorf("developer error: req is nil")
	}

	ctx := r.Context()

	s.injectTools()

	var (
		prompt *string
		err    error
	)
	// Track user prompt if not a small/fast model
	if !s.isSmallFastModel() {
		prompt, err = s.req.LastUserPrompt()
		if err != nil {
			s.logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
		}
	}

	// Add beta header if present in the request.
	var opts []option.RequestOption
	if reqBetaHeader := r.Header.Get("anthropic-beta"); strings.TrimSpace(reqBetaHeader) != "" {
		opts = append(opts, option.WithHeader("anthropic-beta", reqBetaHeader))
	}
	opts = append(opts, option.WithRequestTimeout(time.Second*30))

	client := newAnthropicClient(s.baseURL, s.key, opts...) // TODO: configurable timeout
	messages := s.req.BetaMessageNewParams
	logger := s.logger.With(slog.F("model", s.req.Model))

	var resp *anthropic.BetaMessage

	for {
		resp, err = client.Beta.Messages.New(ctx, messages)
		if err != nil {
			if isConnectionError(err) {
				logger.Warn(ctx, "upstream connection closed", slog.Error(err))
				return xerrors.Errorf("upstream connection closed: %w", err)
			}

			logger.Error(ctx, "anthropic API error", slog.Error(err))
			if antErr := getAnthropicErrorResponse(err); antErr != nil {
				http.Error(w, antErr.Error.Message, antErr.StatusCode)
				return xerrors.Errorf("api error: %w", err)
			}

			logger.Error(ctx, "upstream API error", slog.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return xerrors.Errorf("upstream API error: %w", err)
		}

		if prompt != nil {
			if err := s.tracker.TrackPromptUsage(ctx, s.id, resp.ID, *prompt, nil); err != nil {
				s.logger.Warn(ctx, "failed to track prompt usage", slog.Error(err))
			}
		}

		if err := s.tracker.TrackTokensUsage(ctx, s.id, resp.ID, resp.Usage.InputTokens, resp.Usage.OutputTokens, Metadata{
			"web_search_requests":      resp.Usage.ServerToolUse.WebSearchRequests,
			"cache_creation_input":     resp.Usage.CacheCreationInputTokens,
			"cache_read_input":         resp.Usage.CacheReadInputTokens,
			"cache_ephemeral_1h_input": resp.Usage.CacheCreation.Ephemeral1hInputTokens,
			"cache_ephemeral_5m_input": resp.Usage.CacheCreation.Ephemeral5mInputTokens,
		}); err != nil {
			logger.Warn(ctx, "failed to track token usage", slog.Error(err))
		}

		// Handle tool calls for non-streaming.
		var pendingToolCalls []anthropic.BetaToolUseBlock
		for _, c := range resp.Content {
			toolUse := c.AsToolUse()
			if toolUse.ID == "" {
				continue
			}

			if s.toolMgr.GetTool(toolUse.Name) != nil {
				pendingToolCalls = append(pendingToolCalls, toolUse)
				continue
			}

			// If tool is not injected, track it since the client will be handling it.
			if err := s.tracker.TrackToolUsage(ctx, s.id, resp.ID, toolUse.Name, toolUse.Input, false, nil); err != nil {
				logger.Warn(ctx, "failed to track tool usage", slog.Error(err))
			}
		}

		// If no injected tool calls, we're done.
		if len(pendingToolCalls) == 0 {
			break
		}

		// Append the assistant's message (which contains the tool_use block)
		// to the messages for the next API call.
		messages.Messages = append(messages.Messages, resp.ToParam())

		// Process each pending tool call.
		for _, tc := range pendingToolCalls {
			tool := s.toolMgr.GetTool(tc.Name)
			if tool == nil {
				logger.Warn(ctx, "tool not found in manager", slog.F("tool", tc.Name))
				// Continue to next tool call, but still append an error tool_result
				messages.Messages = append(messages.Messages,
					anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error: tool %s not found", tc.Name), true)),
				)
				continue
			}

			var args map[string]any
			serialized, err := json.Marshal(tc.Input)
			if err != nil {
				logger.Warn(ctx, "failed to marshal tool args for unmarshal", slog.Error(err), slog.F("tool", tc.Name))
				// Continue to next tool call, but still append an error tool_result
				messages.Messages = append(messages.Messages,
					anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error unmarshaling tool arguments: %v", err), true)),
				)
				continue
			} else if err := json.Unmarshal(serialized, &args); err != nil {
				logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err), slog.F("tool", tc.Name))
				// Continue to next tool call, but still append an error tool_result
				messages.Messages = append(messages.Messages,
					anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error unmarshaling tool arguments: %v", err), true)),
				)
				continue
			}

			// Track injected tool usage - strip MCP tool namespacing if possible
			toolName := tc.Name
			if _, tool, err := DecodeToolID(toolName); err == nil {
				toolName = tool
			}
			if err := s.tracker.TrackToolUsage(ctx, s.id, resp.ID, toolName, args, true, nil); err != nil {
				logger.Warn(ctx, "failed to track tool usage", slog.Error(err))
			}

			res, err := tool.Call(ctx, args)
			if err != nil {
				// Always provide a tool_result even if the tool call failed
				messages.Messages = append(messages.Messages,
					anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock(tc.ID, fmt.Sprintf("Error calling tool: %v", err), true)),
				)
				continue
			}

			// Process tool result
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
	}

	if resp == nil {
		return nil
	}

	// Overwrite response identifier since proxy obscures injected tool call invocations.
	resp.ID = s.id

	out, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "error marshaling response", http.StatusInternalServerError)
		return xerrors.Errorf("failed to marshal response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)

	return nil
}

func (s *AnthropicMessagesBlockingSession) Close() error {
	return nil
}
