package aibridged

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

var _ Session[ChatCompletionNewParamsWrapper] = &OpenAIStreamingSession{}

type OpenAISessionBase struct {
	id           string
	baseURL, key string
	logger       slog.Logger
	tracker      Tracker
	toolMgr      ToolManager
}

func (s *OpenAISessionBase) Init(logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) string {
	s.id = uuid.NewString()

	s.logger = logger.With(slog.F("session_id", s.id))

	s.baseURL = baseURL
	s.key = key

	s.tracker = tracker
	s.toolMgr = toolMgr

	return s.id
}

func (*OpenAISessionBase) LastUserPrompt(req ChatCompletionNewParamsWrapper) (*string, error) {
	return req.LastUserPrompt()
}

type OpenAIStreamingSession struct {
	OpenAISessionBase
}

func (s *OpenAIStreamingSession) Init(logger slog.Logger, baseURL, key string, tracker Tracker, toolMgr ToolManager) string {
	return s.OpenAISessionBase.Init(logger.Named("openai.streaming"), baseURL, key, tracker, toolMgr)
}

func (s *OpenAIStreamingSession) Execute(req *ChatCompletionNewParamsWrapper, w http.ResponseWriter, r *http.Request) error {
	if req == nil {
		return xerrors.Errorf("developer error: req is nil")
	}

	// Include token usage.
	req.StreamOptions.IncludeUsage = openai.Bool(true)

	// Inject tools.
	for _, tool := range s.toolMgr.ListTools() {
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

		req.Tools = append(req.Tools, fn)
	}

	// Allow us to interrupt watch via cancel.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx) // Rewire context for SSE cancellation.

	client := newOpenAIClient(s.baseURL, s.key)

	streamCtx, streamCancel := context.WithCancelCause(ctx)
	defer streamCancel(xerrors.New("deferred"))

	events := newEventStream(openAIEventStream)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if err := events.Close(streamCtx); err != nil {
				s.logger.Error(ctx, "error closing stream", slog.Error(err))
			}
		}()

		BasicSSESender(streamCtx, events, s.logger.Named("sse-sender")).ServeHTTP(w, r)
	}()

	// TODO: implement parallel tool calls.
	// TODO: don't send if not supported by model (i.e. o4-mini).
	if len(req.Tools) > 0 { // If no tools are specified but this setting is set, it'll cause a 400 Bad Request.
		req.ParallelToolCalls = openai.Bool(false)
	}

	var (
		stream          *ssestream.Stream[openai.ChatCompletionChunk]
		cumulativeUsage openai.CompletionUsage
	)
	for {
		var pendingToolCalls []openai.FinishedChatCompletionToolCall

		stream = client.Chat.Completions.NewStreaming(ctx, req.ChatCompletionNewParams)
		var acc openai.ChatCompletionAccumulator
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			// fmt.Printf("[in]: %s\n", chunk.RawJSON())

			shouldRelayChunk := true
			if toolCall, ok := acc.JustFinishedToolCall(); ok {
				// Don't intercept and handle builtin tools.
				if s.toolMgr.GetTool(toolCall.Name) != nil {
					pendingToolCalls = append(pendingToolCalls, toolCall)
					// Don't relay this chunk back; we'll handle it transparently.
					shouldRelayChunk = false
				} else {
					s.tracker.TrackToolUsage(ctx, s.id, chunk.ID, s.Model(req), toolCall.Name, toolCall.Arguments, false, nil)
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

		// If the usage information is set, track it.
		// The API will send usage information when the response terminates, which will happen if a tool call is invoked.
		s.tracker.TrackTokensUsage(ctx, s.id, acc.ID, s.Model(req), cumulativeUsage.PromptTokens, cumulativeUsage.CompletionTokens, Metadata{
			"prompt_audio":                   cumulativeUsage.PromptTokensDetails.AudioTokens,
			"prompt_cached":                  cumulativeUsage.PromptTokensDetails.CachedTokens,
			"completion_accepted_prediction": cumulativeUsage.CompletionTokensDetails.AcceptedPredictionTokens,
			"completion_rejected_prediction": cumulativeUsage.CompletionTokensDetails.RejectedPredictionTokens,
			"completion_audio":               cumulativeUsage.CompletionTokensDetails.AudioTokens,
			"completion_reasoning":           cumulativeUsage.CompletionTokensDetails.ReasoningTokens,
		})

		if err := stream.Err(); err != nil {
			s.logger.Error(ctx, "server stream error", slog.Error(err))
			var apierr *openai.Error
			if errors.As(err, &apierr) {
				events.TrySend(ctx, map[string]interface{}{
					// TODO: session ID?
					"error":   true,
					"message": err.Error(),
				})
				// http.Error(w, apierr.Message, apierr.StatusCode)
				break
			} else if isConnectionError(err) {
				s.logger.Warn(ctx, "upstream connection error", slog.Error(err))
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
				// Not an known tool, don't do anything.
				s.logger.Warn(ctx, "pending tool call for non-managed tool, skipping...", slog.F("tool", tc.Name))
				continue
			}

			// Only do this once.
			if !appendedPrevMsg {
				// Append the whole message from this stream as context since we'll be sending a new request with the tool results.
				req.Messages = append(req.Messages, acc.Choices[len(acc.Choices)-1].Message.ToParam())
				appendedPrevMsg = true
			}

			s.tracker.TrackToolUsage(ctx, s.id, acc.ID, s.Model(req), tc.Name, tc.Arguments, true, nil)

			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				s.logger.Warn(ctx, "failed to unmarshal tool args", slog.Error(err), slog.F("tool", tc.Name))
			}

			res, err := tool.Call(streamCtx, args)
			if err != nil {
				// Always provide a tool_result even if the tool call failed
				errorResponse := map[string]interface{}{
					// TODO: session ID?
					"error":   true,
					"message": err.Error(),
				}
				errorJSON, _ := json.Marshal(errorResponse)
				req.Messages = append(req.Messages, openai.ToolMessage(string(errorJSON), tc.ID))
				continue
			}

			var out strings.Builder
			if err := json.NewEncoder(&out).Encode(res); err != nil {
				s.logger.Error(ctx, "failed to encode tool response", slog.Error(err))
				// Always provide a tool_result even if encoding failed
				// TODO: abstract.
				errorResponse := map[string]interface{}{
					// TODO: session ID?
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

	err := events.Close(streamCtx)
	if err != nil {
		s.logger.Error(ctx, "failed to close event stream", slog.Error(err))
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

func (s *OpenAIStreamingSession) Model(req *ChatCompletionNewParamsWrapper) Model {
	var model string
	if req == nil {
		model = "?"
	} else {
		model = req.Model
	}

	return Model{
		Provider:  "openai",
		ModelName: model,
	}
}

func (s *OpenAIStreamingSession) Close() error {
	return nil // TODO: do we even need this?
}
