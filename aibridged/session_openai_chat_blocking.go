package aibridged

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openai/openai-go"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var _ Session = &OpenAIBlockingChatSession{}

type OpenAIBlockingChatSession struct {
	OpenAIChatSessionBase
}

func NewOpenAIBlockingChatSession(req *ChatCompletionNewParamsWrapper, baseURL, key string) *OpenAIBlockingChatSession {
	return &OpenAIBlockingChatSession{OpenAIChatSessionBase: OpenAIChatSessionBase{
		req:     req,
		baseURL: baseURL,
		key:     key,
	}}
}

func (s *OpenAIBlockingChatSession) Init(logger slog.Logger, tracker Tracker, toolMgr ToolManager) string {
	return s.OpenAIChatSessionBase.Init(logger.Named("blocking"), tracker, toolMgr)
}

func (s *OpenAIBlockingChatSession) ProcessRequest(w http.ResponseWriter, r *http.Request) error {
	if s.req == nil {
		return xerrors.Errorf("developer error: req is nil")
	}

	ctx := r.Context()
	client := newOpenAIClient(s.baseURL, s.key)
	logger := s.logger.With(slog.F("model", s.req.Model))

	var cumulativeUsage openai.CompletionUsage
	var (
		completion *openai.ChatCompletion
		err        error
	)

	s.injectTools()

	prompt, err := s.req.LastUserPrompt()
	if err != nil {
		logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
	}

	for {
		completion, err = client.Chat.Completions.New(ctx, s.req.ChatCompletionNewParams)
		if err != nil {
			break
		}

		if prompt != nil {
			if err := s.tracker.TrackPromptUsage(ctx, s.id, completion.ID, *prompt, nil); err != nil {
				logger.Warn(ctx, "failed to track prompt usage", slog.Error(err))
			}
		}

		// Track cumulative usage
		cumulativeUsage = sumUsage(cumulativeUsage, completion.Usage)

		// Track token usage
		if err := s.tracker.TrackTokensUsage(ctx, s.id, completion.ID, cumulativeUsage.PromptTokens, cumulativeUsage.CompletionTokens, Metadata{
			"prompt_audio":                   cumulativeUsage.PromptTokensDetails.AudioTokens,
			"prompt_cached":                  cumulativeUsage.PromptTokensDetails.CachedTokens,
			"completion_accepted_prediction": cumulativeUsage.CompletionTokensDetails.AcceptedPredictionTokens,
			"completion_rejected_prediction": cumulativeUsage.CompletionTokensDetails.RejectedPredictionTokens,
			"completion_audio":               cumulativeUsage.CompletionTokensDetails.AudioTokens,
			"completion_reasoning":           cumulativeUsage.CompletionTokensDetails.ReasoningTokens,
		}); err != nil {
			logger.Warn(ctx, "failed to track tokens usage", slog.Error(err))
		}

		// Check if we have tool calls to process
		var pendingToolCalls []openai.ChatCompletionMessageToolCall
		if len(completion.Choices) > 0 && completion.Choices[0].Message.ToolCalls != nil {
			for _, toolCall := range completion.Choices[0].Message.ToolCalls {
				if s.toolMgr.GetTool(toolCall.Function.Name) != nil {
					pendingToolCalls = append(pendingToolCalls, toolCall)
				} else {
					if err := s.tracker.TrackToolUsage(ctx, s.id, completion.ID, toolCall.Function.Name, toolCall.Function.Arguments, false, nil); err != nil {
						s.logger.Warn(ctx, "failed to track tool usage", slog.Error(err), slog.F("tool", toolCall.Function.Name))
					}
				}
			}
		}

		// If no injected tool calls, we're done
		if len(pendingToolCalls) == 0 {
			break
		}

		appendedPrevMsg := false
		// Process each pending tool call
		for _, tc := range pendingToolCalls {
			tool := s.toolMgr.GetTool(tc.Function.Name)
			if tool == nil {
				// Not a known tool, don't do anything.
				logger.Warn(ctx, "pending tool call for non-managed tool, skipping", slog.F("tool", tc.Function.Name))
				continue
			}
			// Only do this once.
			if !appendedPrevMsg {
				// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
				s.req.Messages = append(s.req.Messages, completion.Choices[0].Message.ToParam())
				appendedPrevMsg = true
			}

			if err := s.tracker.TrackToolUsage(ctx, s.id, completion.ID, tool.Name, tc.Function.Arguments, true, nil); err != nil {
				logger.Warn(ctx, "failed to track tool usage", slog.Error(err), slog.F("tool", tool.Name))
			}

			var (
				args map[string]string
				buf  bytes.Buffer
			)
			_ = json.NewEncoder(&buf).Encode(tc.Function.Arguments)
			_ = json.NewDecoder(&buf).Decode(&args)
			res, err := tool.Call(ctx, args)
			if err != nil {
				// Always provide a tool result even if the tool call failed
				errorResponse := map[string]interface{}{
					// TODO: session ID?
					"error":   true,
					"message": err.Error(),
				}
				errorJSON, _ := json.Marshal(errorResponse)
				s.req.Messages = append(s.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			var out strings.Builder
			if err := json.NewEncoder(&out).Encode(res); err != nil {
				logger.Error(ctx, "failed to encode tool response", slog.Error(err))
				// Always provide a tool result even if encoding failed
				errorResponse := map[string]interface{}{
					// TODO: session ID?
					"error":   true,
					"message": err.Error(),
				}
				errorJSON, _ := json.Marshal(errorResponse)
				s.req.Messages = append(s.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			s.req.Messages = append(s.req.Messages, openai.ToolMessage(out.String(), tc.ID))
		}
	}

	// TODO: these probably have to be formatted as JSON errs?
	if err != nil {
		if isConnectionError(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return xerrors.Errorf("upstream connection closed: %w", err)
		}
		var apierr *openai.Error
		if errors.As(err, &apierr) {
			http.Error(w, apierr.Message, apierr.StatusCode)
			return xerrors.Errorf("api error: %w", apierr)
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return xerrors.Errorf("chat completion failed: %w", err)
	}

	if completion == nil {
		return nil
	}

	// Overwrite response identifier since proxy obscures injected tool call invocations.
	completion.ID = s.id

	// Update the cumulative usage in the final response
	if completion.Usage.CompletionTokens > 0 {
		completion.Usage = cumulativeUsage
	}

	w.Header().Set("Content-Type", "application/json")
	out, err := json.Marshal(completion)
	if err != nil {
		out, _ = json.Marshal(s.newErrorResponse(xerrors.Errorf("failed to marshal response: %w", err)))
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	_, _ = w.Write(out)

	return nil
}
