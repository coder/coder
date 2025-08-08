package aibridged

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var _ Session = &OpenAIStreamingChatSession{}

type OpenAIStreamingChatSession struct {
	OpenAIChatSessionBase
}

func NewOpenAIStreamingChatSession(req *ChatCompletionNewParamsWrapper) *OpenAIStreamingChatSession {
	return &OpenAIStreamingChatSession{OpenAIChatSessionBase: OpenAIChatSessionBase{
		req: req,
	}}
}

func (s *OpenAIStreamingChatSession) Init(id string, logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) {
	s.OpenAIChatSessionBase.Init(id, logger.Named("streaming"), baseURL, key, tracker, toolMgr)
}

func (s *OpenAIStreamingChatSession) ProcessRequest(w http.ResponseWriter, r *http.Request) error {
	if s.req == nil {
		return xerrors.Errorf("developer error: req is nil")
	}

	// Include token usage.
	s.req.StreamOptions.IncludeUsage = openai.Bool(true)

	s.injectTools()

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	client := newOpenAIClient(s.baseURL, s.key)
	logger := s.logger.With(slog.F("model", s.req.Model))

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	events := newEventStream(openAIEventStream)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := events.Close(streamCtx); err != nil {
				logger.Error(ctx, "error closing stream", slog.Error(err))
			}
		}()

		BasicSSESender(streamCtx, events, logger.Named("sse-sender")).ServeHTTP(w, r)
	}()

	// TODO: implement parallel tool calls.
	// TODO: don't send if not supported by model (i.e. o4-mini).
	if len(s.req.Tools) > 0 { // If no tools are specified but this setting is set, it'll cause a 400 Bad Request.
		s.req.ParallelToolCalls = openai.Bool(false)
	}

	prompt, err := s.LastUserPrompt()
	if err != nil {
		logger.Warn(ctx, "failed to retrieve last user prompt", slog.Error(err))
	}

	var (
		stream          *ssestream.Stream[openai.ChatCompletionChunk]
		cumulativeUsage openai.CompletionUsage
	)
	for {
		var pendingToolCalls []openai.FinishedChatCompletionToolCall

		stream = client.Chat.Completions.NewStreaming(ctx, s.req.ChatCompletionNewParams)
		var acc openai.ChatCompletionAccumulator
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			shouldRelayChunk := true
			if toolCall, ok := acc.JustFinishedToolCall(); ok {
				// Don't intercept and handle builtin tools.
				if s.toolMgr.GetTool(toolCall.Name) != nil {
					pendingToolCalls = append(pendingToolCalls, toolCall)
					// Don't relay this chunk back; we'll handle it transparently.
					shouldRelayChunk = false
				} else {
					if err := s.tracker.TrackToolUsage(ctx, s.id, chunk.ID, s.Model(), toolCall.Name, toolCall.Arguments, false, nil); err != nil {
						logger.Warn(ctx, "failed to track tool usage", slog.Error(err))
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

				// Overwrite response identifier since proxy obscures injected tool call invocations.
				chunk.ID = s.id
				events.TrySend(ctx, chunk)
			}
		}

		if prompt != nil {
			if err := s.tracker.TrackPromptUsage(ctx, s.id, acc.ID, s.Model(), *prompt, nil); err != nil {
				logger.Warn(ctx, "failed to track prompt usage", slog.Error(err))
			}
		}

		// If the usage information is set, track it.
		// The API will send usage information when the response terminates, which will happen if a tool call is invoked.
		if err := s.tracker.TrackTokensUsage(ctx, s.id, acc.ID, s.Model(), cumulativeUsage.PromptTokens, cumulativeUsage.CompletionTokens, Metadata{
			"prompt_audio":                   cumulativeUsage.PromptTokensDetails.AudioTokens,
			"prompt_cached":                  cumulativeUsage.PromptTokensDetails.CachedTokens,
			"completion_accepted_prediction": cumulativeUsage.CompletionTokensDetails.AcceptedPredictionTokens,
			"completion_rejected_prediction": cumulativeUsage.CompletionTokensDetails.RejectedPredictionTokens,
			"completion_audio":               cumulativeUsage.CompletionTokensDetails.AudioTokens,
			"completion_reasoning":           cumulativeUsage.CompletionTokensDetails.ReasoningTokens,
		}); err != nil {
			logger.Warn(ctx, "failed to track tokens usage", slog.Error(err))
		}

		if err := stream.Err(); err != nil {
			logger.Error(ctx, "server stream error", slog.Error(err))
			var apierr *openai.Error
			if errors.As(err, &apierr) {
				events.TrySend(ctx, s.newErrorResponse(err))
				break
			} else if isConnectionError(err) {
				logger.Warn(ctx, "upstream connection error", slog.Error(err))
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		if len(pendingToolCalls) == 0 {
			break
		}

		appendedPrevMsg := false
		for _, tc := range pendingToolCalls {
			tool := s.toolMgr.GetTool(tc.Name)
			if tool == nil {
				// Not a known tool, don't do anything.
				logger.Warn(ctx, "pending tool call for non-managed tool, skipping", slog.F("tool", tc.Name))
				continue
			}

			// Only do this once.
			if !appendedPrevMsg {
				// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
				s.req.Messages = append(s.req.Messages, acc.Choices[len(acc.Choices)-1].Message.ToParam())
				appendedPrevMsg = true
			}

			var toolName string
			_, toolName, err = DecodeToolID(tc.Name)
			if err != nil {
				logger.Debug(ctx, "failed to decode tool ID", slog.Error(err), slog.F("name", tc.Name))
				toolName = tc.Name
			}

			if err := s.tracker.TrackToolUsage(ctx, s.id, acc.ID, s.Model(), toolName, tc.Arguments, true, nil); err != nil {
				logger.Warn(ctx, "failed to track tool usage", slog.Error(err), slog.F("tool", tool.Name))
			}

			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err), slog.F("tool", toolName))
			}

			res, err := tool.Call(streamCtx, args)
			if err != nil {
				// Always provide a tool_result even if the tool call failed.
				errorJSON, _ := json.Marshal(s.newErrorResponse(err))
				s.req.Messages = append(s.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			var out strings.Builder
			if err := json.NewEncoder(&out).Encode(res); err != nil {
				logger.Error(ctx, "failed to encode tool response", slog.Error(err))
				// Always provide a tool_result even if encoding failed.
				errorJSON, _ := json.Marshal(s.newErrorResponse(err))
				s.req.Messages = append(s.req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			s.req.Messages = append(s.req.Messages, openai.ToolMessage(out.String(), tc.ID))
		}
	}

	err = events.Close(streamCtx)
	if err != nil {
		logger.Error(ctx, "failed to close event stream", slog.Error(err))
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
	return nil
}

func (s *OpenAIStreamingChatSession) Close() error {
	return nil // TODO: do we even need this?
}
