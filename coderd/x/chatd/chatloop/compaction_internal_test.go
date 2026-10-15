package chatloop

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func compactionStream(parts ...fantasy.StreamPart) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	}
}

func summaryTextStream() fantasy.StreamResponse {
	return compactionStream(
		fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
		fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "summary"},
		fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
		fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, ID: "msg_summary", FinishReason: fantasy.FinishReasonStop},
	)
}

func TestStartCompactionDebugRun_DoesNotReportDebugErrors(t *testing.T) {
	t.Parallel()

	newParentContext := func(chatID uuid.UUID) context.Context {
		return chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
			RunID:               uuid.New(),
			ChatID:              chatID,
			RootChatID:          uuid.New(),
			ParentChatID:        uuid.New(),
			ModelConfigID:       uuid.New(),
			TriggerMessageID:    41,
			HistoryTipMessageID: 42,
			Provider:            "fake-provider",
			Model:               "fake-model",
		})
	}

	t.Run("CreateRun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{}, xerrors.New("insert compaction debug run"))

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
		})
		require.Same(t, ctx, compactionCtx)
		finish(nil)
	})

	t.Run("FinalizeRunAggregatesSummary", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()
		runID := uuid.New()
		usageJSON, err := json.Marshal(fantasy.Usage{InputTokens: 7, OutputTokens: 3})
		require.NoError(t, err)
		attemptsJSON, err := json.Marshal([]chatdebug.Attempt{{
			Status: "completed",
			Method: "POST",
			Path:   "/v1/messages",
		}})
		require.NoError(t, err)

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{ //nolint:exhaustruct // Test only needs IDs.
			ID:     runID,
			ChatID: chatID,
		}, nil)
		db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return([]database.ChatDebugStep{{
			ID:       uuid.New(),
			RunID:    runID,
			ChatID:   chatID,
			Status:   string(chatdebug.StatusCompleted),
			Usage:    pqtype.NullRawMessage{RawMessage: usageJSON, Valid: true},
			Attempts: attemptsJSON,
		}}, nil)
		db.EXPECT().UpdateChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
		).DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, runID, params.ID)
			require.True(t, params.Summary.Valid)
			require.JSONEq(t, `{"endpoint_label":"POST /v1/messages","step_count":1,"total_input_tokens":7,"total_output_tokens":3}`,
				string(params.Summary.RawMessage))
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		})

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
		})
		require.NotSame(t, ctx, compactionCtx)
		finish(nil)
	})

	t.Run("FinalizeRun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()
		runID := uuid.New()

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{ //nolint:exhaustruct // Test only needs IDs.
			ID:     runID,
			ChatID: chatID,
		}, nil)
		db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, xerrors.New("aggregate compaction debug run"))
		db.EXPECT().UpdateChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
		).Return(database.ChatDebugRun{}, xerrors.New("finalize compaction debug run"))

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
		})
		require.NotSame(t, ctx, compactionCtx)
		finish(nil)
	})
}

// TestGenerateCompactionSummary_PanicFinalizesAsError verifies that a
// panic originating inside the model call during compaction is
// captured by the deferred debug-run finalizer so the run is recorded
// with StatusError rather than StatusCompleted. Without the recover
// hook the named `err` return is still nil when the defer fires and
// the row silently misclassifies the crash path.
func TestGenerateCompactionSummary_PanicFinalizesAsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	chatID := uuid.New()
	runID := uuid.New()

	status := make(chan string, 1)

	db.EXPECT().InsertChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
	).Return(database.ChatDebugRun{
		ID:     runID,
		ChatID: chatID,
	}, nil)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil)
	db.EXPECT().UpdateChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
	).DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
		status <- params.Status.String
		return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
	})

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			panic("compaction model crash")
		},
	}

	parentCtx := chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
		RunID:               uuid.New(),
		ChatID:              chatID,
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    1,
		HistoryTipMessageID: 2,
		Provider:            "fake",
		Model:               "fake-model",
	})

	require.PanicsWithValue(t, "compaction model crash", func() {
		_, _, _ = generateCompactionSummary(parentCtx, model,
			[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
			CompactionOptions{
				DebugSvc:      svc,
				ChatID:        chatID,
				SummaryPrompt: "summarize",
			})
	})

	select {
	case s := <-status:
		require.Equal(t, string(chatdebug.StatusError), s,
			"panic path must finalize the debug run with StatusError")
	case <-time.After(testutil.WaitShort):
		t.Fatal("FinalizeRun never reached UpdateChatDebugRun on panic")
	}
}

func TestGenerateCompactionSummaryPreservesCallOptions(t *testing.T) {
	t.Parallel()

	temperature := 0.2
	topP := 0.8
	topK := int64(40)
	presencePenalty := 0.1
	frequencyPenalty := 0.3
	reasoningEffort := fantasyopenai.ReasoningEffortMedium
	providerOptions := fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
			ReasoningEffort: &reasoningEffort,
		},
	}
	toolDefinitions := []fantasy.Tool{
		fantasy.FunctionTool{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
		fantasy.ProviderDefinedTool{ID: "web_search", Name: "web_search"},
	}
	messages := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "system prefix"),
		textMessage(fantasy.MessageRoleUser, "hello"),
	}
	originalMessages := append([]fantasy.Message(nil), messages...)
	var got fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			got = call
			return summaryTextStream(), nil
		},
	}

	toolChoice := fantasy.ToolChoiceNone
	summary, _, err := generateCompactionSummary(context.Background(), model, messages, CompactionOptions{
		SummaryPrompt: "summarize",
		SummaryCall: fantasy.Call{
			Temperature:      &temperature,
			TopP:             &topP,
			TopK:             &topK,
			PresencePenalty:  &presencePenalty,
			FrequencyPenalty: &frequencyPenalty,
			ProviderOptions:  providerOptions,
			ToolChoice:       &toolChoice,
		},
		ToolDefinitions: toolDefinitions,
	})
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	require.Equal(t, &temperature, got.Temperature)
	require.Equal(t, &topP, got.TopP)
	require.Equal(t, &topK, got.TopK)
	require.Equal(t, &presencePenalty, got.PresencePenalty)
	require.Equal(t, &frequencyPenalty, got.FrequencyPenalty)
	require.Equal(t, providerOptions, got.ProviderOptions)
	require.Equal(t, toolDefinitions, got.Tools)
	require.NotNil(t, got.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *got.ToolChoice)
	require.Nil(t, got.MaxOutputTokens)
	require.Len(t, got.Prompt, 3)
	require.Equal(t, originalMessages, []fantasy.Message(got.Prompt[:2]))
	require.Equal(t, fantasy.MessageRoleUser, got.Prompt[2].Role)
	require.Equal(t, []fantasy.MessagePart{fantasy.TextPart{Text: "summarize"}}, got.Prompt[2].Content)
	require.Equal(t, originalMessages, messages)
}

func TestGenerateCompactionSummaryUsesToolDefinitions(t *testing.T) {
	t.Parallel()

	toolDefinitions := []fantasy.Tool{
		fantasy.FunctionTool{
			Name:        "read_file",
			Description: "Read a file.",
			InputSchema: map[string]any{"type": "object"},
		},
		fantasy.ProviderDefinedTool{ID: "web_search", Name: "web_search"},
	}
	messages := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}
	originalMessages := append([]fantasy.Message(nil), messages...)
	var got fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			got = call
			return summaryTextStream(), nil
		},
	}

	toolChoice := fantasy.ToolChoiceNone
	summary, _, err := generateCompactionSummary(context.Background(), model, messages, CompactionOptions{
		SummaryPrompt:   "summarize",
		SummaryCall:     fantasy.Call{ToolChoice: &toolChoice},
		ToolDefinitions: toolDefinitions,
	})
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	require.Equal(t, toolDefinitions, got.Tools)
	require.NotNil(t, got.ToolChoice)
	require.Equal(t, toolChoice, *got.ToolChoice)
	require.Len(t, got.Prompt, 2)
	require.Equal(t, originalMessages, messages)
}

func TestGenerateCompactionSummaryAppliesAnthropicPromptCaching(t *testing.T) {
	t.Parallel()

	messages := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "system"),
		textMessage(fantasy.MessageRoleUser, "hello"),
		textMessage(fantasy.MessageRoleAssistant, "hi"),
	}
	originalMessages := append([]fantasy.Message(nil), messages...)
	var got fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		ModelName:    "claude",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			got = call
			return summaryTextStream(), nil
		},
	}

	_, _, err := generateCompactionSummary(context.Background(), model, messages, CompactionOptions{
		SummaryPrompt: "summarize",
	})
	require.NoError(t, err)
	require.Len(t, got.Prompt, 4)
	cacheControl := fantasy.ProviderOptions{
		fantasyanthropic.Name: &fantasyanthropic.ProviderCacheControlOptions{
			CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
		},
	}
	// Breakpoints land on the system message and the final two messages.
	require.Equal(t, cacheControl, got.Prompt[0].ProviderOptions)
	require.Nil(t, got.Prompt[1].ProviderOptions)
	require.Equal(t, cacheControl, got.Prompt[2].ProviderOptions)
	require.Equal(t, cacheControl, got.Prompt[3].ProviderOptions)
	require.Equal(t, originalMessages, messages)
}

func TestGenerateCompactionSummaryRetriesWithoutToolsWhenContextTooLarge(t *testing.T) {
	t.Parallel()

	toolDefinitions := []fantasy.Tool{
		fantasy.FunctionTool{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
	}
	toolChoice := fantasy.ToolChoiceNone
	var calls []fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			calls = append(calls, call)
			if len(call.Tools) > 0 {
				return nil, &fantasy.ProviderError{
					Title:      "bad request",
					Message:    "Your input exceeds the context window of this model. Please adjust your input and try again.",
					StatusCode: http.StatusBadRequest,
				}
			}
			return summaryTextStream(), nil
		},
	}

	summary, responseID, err := generateCompactionSummary(context.Background(), model,
		[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		CompactionOptions{
			SummaryPrompt:   "summarize",
			SummaryCall:     fantasy.Call{ToolChoice: &toolChoice},
			ToolDefinitions: toolDefinitions,
		},
	)
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	require.Equal(t, "msg_summary", responseID)
	require.Len(t, calls, 2)
	require.Equal(t, toolDefinitions, calls[0].Tools)
	require.Nil(t, calls[1].Tools)
	require.Equal(t, calls[0].Prompt, calls[1].Prompt)
	require.Equal(t, calls[0].ToolChoice, calls[1].ToolChoice)
}

func TestGenerateCompactionSummaryDoesNotRetryOtherFailures(t *testing.T) {
	t.Parallel()

	contextTooLarge := &fantasy.ProviderError{
		Message:    "Your input exceeds the context window of this model.",
		StatusCode: http.StatusBadRequest,
	}
	cases := []struct {
		name            string
		toolDefinitions []fantasy.Tool
		err             error
		rejected        []string
	}{
		{
			name:            "unrelated bad request",
			toolDefinitions: []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}},
			err: &fantasy.ProviderError{
				Message:    "Unsupported parameter: 'temperature' is not supported with this model.",
				StatusCode: http.StatusBadRequest,
			},
		},
		{
			name:            "server error",
			toolDefinitions: []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}},
			err: &fantasy.ProviderError{
				Message:    "context window service unavailable",
				StatusCode: http.StatusServiceUnavailable,
			},
		},
		{
			name:            "non-provider error",
			toolDefinitions: []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}},
			err:             xerrors.New("context window"),
		},
		{
			name:     "no tools to drop",
			err:      contextTooLarge,
			rejected: []string{"full"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			model := &chattest.FakeModel{
				ProviderName: "fake",
				ModelName:    "fake-model",
				StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					calls++
					return nil, tc.err
				},
			}
			logSink := testutil.NewFakeSink(t)

			_, _, err := generateCompactionSummary(context.Background(), model,
				[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
				CompactionOptions{
					SummaryPrompt:   "summarize",
					ToolDefinitions: tc.toolDefinitions,
					Logger:          logSink.Logger(),
				},
			)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, 1, calls)
			require.Equal(t, tc.rejected, rejectedSummaryRequestKinds(t, logSink))
		})
	}
}

func contextWindowError() error {
	return &fantasy.ProviderError{
		Title:      "bad request",
		Message:    "Your input exceeds the context window of this model. Please adjust your input and try again.",
		StatusCode: http.StatusBadRequest,
	}
}

// toolResultHistory returns a user message followed by one read_file call
// per result, each answered with that result.
func toolResultHistory(results ...string) []fantasy.Message {
	messages := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}
	for i, result := range results {
		callID := fmt.Sprintf("call-%d", i)
		messages = append(messages,
			fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: callID, ToolName: "read_file", Input: "{}"}},
			},
			fantasy.Message{
				Role:    fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{fantasy.ToolResultPart{ToolCallID: callID, Output: fantasy.ToolResultOutputContentText{Text: result}}},
			},
		)
	}
	return messages
}

func toolResultTexts(prompt []fantasy.Message) []string {
	var texts []string
	for _, msg := range prompt {
		for _, part := range msg.Content {
			result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if !ok {
				continue
			}
			if output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output); ok {
				texts = append(texts, output.Text)
			}
		}
	}
	return texts
}

// TestGenerateCompactionSummaryElidesToolResultsWhenStillTooLarge covers a
// history the provider rejects even without tool definitions: the third
// request cuts the large tool result, the caller's messages stay intact,
// and the compaction span describes the request that fit.
func TestGenerateCompactionSummaryElidesToolResultsWhenStillTooLarge(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("a", 60_000)
	messages := toolResultHistory(large, "small result")
	toolDefinitions := []fantasy.Tool{
		fantasy.FunctionTool{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
	}
	var calls []fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			calls = append(calls, call)
			// The provider's real window fits the history only once the
			// large result is cut.
			if slices.Contains(toolResultTexts(call.Prompt), large) {
				return nil, contextWindowError()
			}
			return summaryTextStream(), nil
		},
	}
	ctx, spanAttr := compactionSpan(t)
	logSink := testutil.NewFakeSink(t)

	summary, _, err := generateCompactionSummary(ctx, model, messages, CompactionOptions{
		SummaryPrompt:   "summarize",
		ToolDefinitions: toolDefinitions,
		Logger:          logSink.Logger(),
	})
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	require.Len(t, calls, 3)
	require.Equal(t, toolDefinitions, calls[0].Tools)
	require.Nil(t, calls[1].Tools)
	require.Equal(t, calls[0].Prompt, calls[1].Prompt)
	require.Nil(t, calls[2].Tools)
	texts := toolResultTexts(calls[2].Prompt)
	require.Len(t, texts, 2)
	require.Contains(t, texts[0], "Coder truncated")
	require.Equal(t, "small result", texts[1])
	elidedHistory := calls[2].Prompt[:len(calls[2].Prompt)-1]
	require.LessOrEqual(t, estimateMessagesTokens(elidedHistory), estimateMessagesTokens(messages)/2)
	require.Equal(t, toolResultHistory(large, "small result"), messages, "the caller's messages must not change")
	require.Equal(t, []string{"full", "without_tools"}, rejectedSummaryRequestKinds(t, logSink))

	require.EqualValues(t, 3, spanAttr(AttrSummaryRequestsSent).AsInt64())
	require.Equal(t, "elided", spanAttr(AttrSummaryRequestKind).AsString())
	require.Zero(t, spanAttr(AttrSummaryToolCount).AsInt64())
	require.EqualValues(t, len(large)-len(texts[0]), spanAttr(AttrSummaryElidedBytes).AsInt64())
}

// TestGenerateCompactionSummaryElidesToFitSummaryWindow covers a history
// larger than the summary model's window on a provider that rejects input
// plus max_tokens above the window: the elided request must fit the
// window rather than half the history, and its cap the room left. The
// summary prompt and hint share that window, so the history gets half of
// what they leave.
func TestGenerateCompactionSummaryElidesToFitSummaryWindow(t *testing.T) {
	t.Parallel()

	const window = 9_000
	cases := []struct {
		name string
		hint string
	}{
		{name: "without a hint"},
		// The largest model context a pre_compact hook can return.
		{name: "with the largest hook hint", hint: strings.Repeat("h", 16<<10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			messages := toolResultHistory(strings.Repeat("a", 60_000))
			summaryTokens := estimateTokens(len("summarize") + len(tc.hint))
			var calls []fantasy.Call
			model := &chattest.FakeModel{
				ProviderName: "fake",
				ModelName:    "fake-model",
				StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
					calls = append(calls, call)
					if estimateMessagesTokens(call.Prompt)+ptr.NilToEmpty(call.MaxOutputTokens) > window {
						return nil, contextWindowError()
					}
					return summaryTextStream(), nil
				},
			}
			ctx, spanAttr := compactionSpan(t)

			summary, _, err := generateCompactionSummary(ctx, model, messages, CompactionOptions{
				SummaryPrompt:        "summarize",
				SummaryHint:          tc.hint,
				SummaryCall:          fantasy.Call{MaxOutputTokens: ptr.Ref(int64(8_000))},
				ToolDefinitions:      []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}},
				summaryWindow:        window,
				estimatedInputTokens: estimateMessagesTokens(messages) + summaryTokens,
			})
			require.NoError(t, err)
			require.Equal(t, "summary", summary)
			require.Len(t, calls, 3)
			require.Nil(t, calls[2].Tools)
			elidedHistory := estimateMessagesTokens(calls[2].Prompt[:len(calls[2].Prompt)-1])
			require.LessOrEqual(t, elidedHistory, (window-summaryTokens)/2)
			elidedInput := elidedHistory + summaryTokens
			require.Equal(t, window-elidedInput, *calls[2].MaxOutputTokens)

			require.Equal(t, "elided", spanAttr(AttrSummaryRequestKind).AsString())
			require.Equal(t, elidedInput, spanAttr(AttrSummaryEstimatedInputTokens).AsInt64())
			require.Equal(t, window-elidedInput, spanAttr(AttrSummaryMaxOutputTokens).AsInt64())
		})
	}
}

// compactionSpan returns a context carrying a recording span and a lookup
// of the span's attributes. The first lookup ends the span.
func compactionSpan(t *testing.T) (context.Context, func(attribute.Key) attribute.Value) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, tracerProvider.Shutdown(context.Background()))
	})
	ctx, span := tracerProvider.Tracer("test").Start(context.Background(), "compaction")
	return ctx, func(key attribute.Key) attribute.Value {
		t.Helper()
		span.End()
		ended := recorder.Ended()
		require.Len(t, ended, 1)
		attrs := attribute.NewSet(ended[0].Attributes()...)
		value, ok := attrs.Value(key)
		require.True(t, ok, "missing span attribute %s", key)
		return value
	}
}

// rejectedSummaryRequestKinds returns the kind of each summary request the
// sink recorded as rejected for its size.
func rejectedSummaryRequestKinds(t *testing.T, sink *testutil.FakeSink) []string {
	t.Helper()

	var kinds []string
	for _, entry := range sink.Entries(func(entry slog.SinkEntry) bool {
		return entry.Message == "compaction summary request rejected for its size"
	}) {
		require.Equal(t, slog.LevelWarn, entry.Level)
		for _, field := range entry.Fields {
			if field.Name == AttrSummaryRequestKind {
				kinds = append(kinds, fmt.Sprint(field.Value))
			}
		}
	}
	return kinds
}

// TestGenerateCompactionSummaryReportsConversationTooLarge covers a history
// no request fits: every request shape is tried, and the rejection is
// classified as a non-retryable configuration error that tells the user how
// to recover.
func TestGenerateCompactionSummaryReportsConversationTooLarge(t *testing.T) {
	t.Parallel()

	rejection := contextWindowError()
	calls := 0
	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			calls++
			return nil, rejection
		},
	}
	logSink := testutil.NewFakeSink(t)

	_, _, err := generateCompactionSummary(context.Background(), model,
		toolResultHistory(strings.Repeat("a", 60_000)),
		CompactionOptions{
			SummaryPrompt:    "summarize",
			ResolvedProvider: "openai",
			ToolDefinitions:  []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}},
			Logger:           logSink.Logger(),
		},
	)
	require.ErrorIs(t, err, rejection)
	require.Equal(t, 3, calls)
	require.Equal(t, []string{"full", "without_tools", "elided"}, rejectedSummaryRequestKinds(t, logSink))
	classified := chaterror.Classify(err)
	require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	require.Equal(t, compactionContextTooLargeMessage, classified.Message)
	require.False(t, classified.Retryable)
	require.Equal(t, "openai", classified.Provider)
}

// TestGenerateCompactionSummaryRetriesInStreamOverflow drives the OpenAI
// Responses client through each shape a context-window rejection takes
// inside an HTTP 200 stream. Bedrock Mantle sends the error envelope.
func TestGenerateCompactionSummaryRetriesInStreamOverflow(t *testing.T) {
	t.Parallel()

	const overflow = "Your input exceeds the context window of this model. Please adjust your input and try again."
	cases := []struct {
		name  string
		event string
	}{
		{
			name:  "error envelope",
			event: `{"error":{"code":"context_length_exceeded","message":"` + overflow + `","type":"invalid_request_error"}}`,
		},
		{
			name:  "error event",
			event: `{"type":"error","code":"context_length_exceeded","message":"` + overflow + `","param":null,"sequence_number":0}`,
		},
		{
			name:  "response failed event",
			event: `{"type":"response.failed","sequence_number":0,"response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"context_length_exceeded","message":"` + overflow + `"}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			var toolCounts []int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.True(t, strings.HasSuffix(r.URL.Path, "/responses"), "unexpected path %s", r.URL.Path)
				var body struct {
					Tools []json.RawMessage `json:"tools"`
				}
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}
				mu.Lock()
				toolCounts = append(toolCounts, len(body.Tools))
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				_, err := fmt.Fprintf(w, "data: %s\n\n", tc.event)
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)
			client, err := fantasyopenai.New(
				fantasyopenai.WithAPIKey("test-key"),
				fantasyopenai.WithBaseURL(server.URL),
				fantasyopenai.WithUseResponsesAPI(),
			)
			require.NoError(t, err)
			model, err := client.LanguageModel(t.Context(), "gpt-4.1")
			require.NoError(t, err)

			_, _, err = generateCompactionSummary(t.Context(), model,
				[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
				CompactionOptions{
					SummaryPrompt:    "summarize",
					ResolvedProvider: "openai",
					ToolDefinitions: []fantasy.Tool{
						fantasy.FunctionTool{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
					},
				},
			)
			require.Error(t, err)
			require.Equal(t, codersdk.ChatErrorKindConfig, chaterror.Classify(err).Kind)
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, []int{1, 0}, toolCounts, "the rejected request is retried without tools")
		})
	}
}

func TestIsContextTooLargeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "parsed by fantasy",
			err: &fantasy.ProviderError{
				StatusCode:         http.StatusBadRequest,
				ContextTooLargeErr: true,
				ContextMaxTokens:   16385,
				ContextUsedTokens:  20000,
			},
			want: true,
		},
		{
			name: "openai responses wording",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "Your input exceeds the context window of this model. Please adjust your input and try again.",
			},
			want: true,
		},
		{
			name: "anthropic wording",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "prompt is too long: 213462 tokens > 200000 maximum",
			},
			want: true,
		},
		{
			name: "wording in another case",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "Your Input Exceeds The Context Window Of This Model.",
			},
			want: true,
		},
		{
			name: "openai error code in response body",
			err: &fantasy.ProviderError{
				StatusCode:   http.StatusBadRequest,
				Message:      "Request too large for this model.",
				ResponseBody: []byte(`{"error":{"message":"Request too large for this model.","type":"invalid_request_error","code":"context_length_exceeded"}}`),
			},
			want: true,
		},
		{
			name: "wrapped provider error",
			err: xerrors.Errorf("generate summary text: %w", &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "This model's maximum context length is 16385 tokens.",
			}),
			want: true,
		},
		{
			name: "request entity too large",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusRequestEntityTooLarge,
				Message:    "Request Entity Too Large",
			},
			want: true,
		},
		{
			name: "anthropic input plus max_tokens wording",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "input length and `max_tokens` exceed context limit: 188240 + 21333 > 200000, decrease input length or `max_tokens` and try again",
			},
			want: true,
		},
		{
			// An error event inside an HTTP 200 stream carries no status.
			name: "in-stream error",
			err: &fantasy.ProviderError{
				Title:        "stream error",
				Message:      "Your input exceeds the context window of this model. Please adjust your input and try again.",
				ResponseBody: []byte(`{"error":{"code":"context_length_exceeded","message":"Your input exceeds the context window of this model. Please adjust your input and try again.","type":"invalid_request_error"}}`),
			},
			want: true,
		},
		{
			name: "in-stream error code in response body",
			err: &fantasy.ProviderError{
				Title:        "stream error",
				Message:      "Invalid request.",
				ResponseBody: []byte(`{"error":{"code":"context_length_exceeded","message":"Invalid request."}}`),
			},
			want: true,
		},
		{
			name: "transient in-stream error",
			err: &fantasy.ProviderError{
				Title:          "stream error",
				Message:        "context window service unavailable",
				TransientError: true,
			},
			want: false,
		},
		{
			name: "unrelated in-stream error",
			err: &fantasy.ProviderError{
				Title:        "stream error",
				Message:      "The server had an error while processing your request.",
				ResponseBody: []byte(`{"error":{"code":"server_error","message":"The server had an error while processing your request.","type":"server_error"}}`),
			},
			want: false,
		},
		{
			name: "responses error event",
			err: &fantasy.Error{
				Title:   "response error",
				Message: "Your input exceeds the context window of this model. (code: context_length_exceeded)",
			},
			want: true,
		},
		{
			name: "responses error event without a code",
			err: &fantasy.Error{
				Title:   "response error",
				Message: "Your input exceeds the context window of this model.",
			},
			want: true,
		},
		{
			name: "unrelated responses failure",
			err: &fantasy.Error{
				Title:   "response failed",
				Message: "The server had an error while processing your request. (code: server_error)",
			},
			want: false,
		},
		{
			name: "responses failure with matching wording and another code",
			err: &fantasy.Error{
				Title:   "response failed",
				Message: "context window tokens per minute exceeded (code: rate_limit_exceeded)",
			},
			want: false,
		},
		{
			name: "unrelated bad request",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid schema for function 'read_file'.",
			},
			want: false,
		},
		{
			name: "matching wording on a non bad-request status",
			err: &fantasy.ProviderError{
				StatusCode: http.StatusTooManyRequests,
				Message:    "context window tokens per minute exceeded",
			},
			want: false,
		},
		{
			name: "non-provider error",
			err:  xerrors.New("prompt is too long"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, isContextTooLargeError(tc.err))
		})
	}
}

func TestGenerateCompactionSummary_UsesCallerContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	testCtx := context.WithValue(context.Background(), contextKey("key"), "value")
	var (
		ctxSeen   context.Context
		errAtCall error
	)
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			ctxSeen = ctx
			errAtCall = ctx.Err()
			return compactionStream(
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "summary"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			), nil
		},
	}

	summary, _, err := generateCompactionSummary(testCtx, model,
		[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		CompactionOptions{SummaryPrompt: "summarize"},
	)
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	// The silence guard wraps the caller context in a cancelable child that
	// is released after the summary completes, so assert inheritance rather
	// than identity: values propagate, the context is live at call time, and
	// no deadline is attached (the guard uses a timer, not a context
	// deadline).
	require.NoError(t, errAtCall)
	_, ok := ctxSeen.Deadline()
	require.False(t, ok)
	require.Equal(t, "value", ctxSeen.Value(contextKey("key")))
}

func TestGenerateCompactionSummary_Stream(t *testing.T) {
	t.Parallel()

	t.Run("joins text blocks", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "first"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "first", Delta: " first "},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "first"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "second"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "second", Delta: " second "},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "second"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				), nil
			},
		}

		summary, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{})
		require.NoError(t, err)
		require.Equal(t, "first second", summary)
	})

	t.Run("reports output cap truncation", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningStart, ID: "reasoning"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning", Delta: "thinking"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningEnd, ID: "reasoning"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonLength},
				), nil
			},
		}

		summary, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{})
		require.Empty(t, summary)
		require.EqualError(t, err, "compaction summary was truncated at the output token cap")
	})

	t.Run("rejects stream without finish part", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "partial summary"},
				), nil
			},
		}

		summary, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{})
		require.Empty(t, summary)
		require.EqualError(t, err, "compaction summary stream ended without a finish part")
	})

	t.Run("classifies canceled stream open as transport reset", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return nil, context.Canceled
			},
		}

		summary, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{
			ResolvedProvider: "fake",
		})
		require.Empty(t, summary)
		require.ErrorIs(t, err, chaterror.ErrProviderTransportReset)
		classified := chaterror.Classify(err)
		require.True(t, classified.Retryable)
		require.Equal(t, "fake", classified.Provider)
	})

	t.Run("classifies canceled error part as transport reset", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: context.Canceled},
				), nil
			},
		}

		summary, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{
			ResolvedProvider: "fake",
		})
		require.Empty(t, summary)
		require.ErrorIs(t, err, chaterror.ErrProviderTransportReset)
		classified := chaterror.Classify(err)
		require.True(t, classified.Retryable)
		require.Equal(t, "fake", classified.Provider)
	})

	t.Run("classifies stream silence", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()
		model := &chattest.FakeModel{
			ProviderName: "openai",
			StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return func(yield func(fantasy.StreamPart) bool) {
					<-ctx.Done()
				}, nil
			},
		}

		done := make(chan error, 1)
		go func() {
			_, _, err := generateCompactionSummary(context.Background(), model, nil, CompactionOptions{
				ResolvedProvider:     "openai",
				Clock:                clock,
				StreamSilenceTimeout: 5 * time.Millisecond,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		err := <-done
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindStreamSilenceTimeout, classified.Kind)
		require.Equal(t, "openai", classified.Provider)
		require.True(t, classified.Retryable)
	})
}

// TestGenerateCompaction_ForceBypassesThresholdGates verifies the
// manual-compaction contract: Force runs the summary even when usage
// is below threshold, when usage is zero, and when threshold=100
// disables automatic compaction; without Force those gates return an
// empty result without calling the model.
func TestGenerateCompaction_ForceBypassesThresholdGates(t *testing.T) {
	t.Parallel()

	newModel := func(calls *int) *chattest.FakeModel {
		return &chattest.FakeModel{
			ProviderName: "fake",
			ModelName:    "fake-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				*calls++
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "forced summary"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				), nil
			},
		}
	}
	messages := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}

	cases := []struct {
		name string
		opts GenerateCompactionOptions
	}{
		{
			name: "below threshold",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 70,
				ContextLimit:     1000,
				StepUsage:        fantasy.Usage{InputTokens: 10},
			},
		},
		{
			name: "zero usage",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 70,
				ContextLimit:     1000,
			},
		},
		{
			name: "threshold disabled",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 100,
				ContextLimit:     1000,
				StepUsage:        fantasy.Usage{InputTokens: 10},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Without Force the gate returns an empty result and
			// never calls the model.
			calls := 0
			opts := tc.opts
			opts.Model = newModel(&calls)
			opts.Messages = messages
			opts.Clock = quartz.NewMock(t)
			result, err := GenerateCompaction(context.Background(), opts)
			require.NoError(t, err)
			require.Empty(t, result.SummaryReport)
			require.Zero(t, calls, "gated run must not call the model")

			// With Force the summary is generated and labeled manual.
			opts.Force = true
			opts.Source = CompactionSourceManual
			result, err = GenerateCompaction(context.Background(), opts)
			require.NoError(t, err)
			require.Equal(t, "forced summary", result.SummaryReport)
			require.Equal(t, CompactionSourceManual, result.Source)
			require.Equal(t, 1, calls, "forced run calls the model once")
		})
	}
}

// TestGenerateCompaction_ClampsSummaryCapToRemainingWindow pins the
// summary output cap bound: sum-enforcing providers reject requests
// whose input plus max_tokens exceeds the context window, so the cap
// shrinks to the remaining window minus the trigger step's output, the
// trailing messages, and the appended summary prompt (ceil(bytes/3)
// tokens), and degenerate cases stay unchanged.
func TestGenerateCompaction_ClampsSummaryCapToRemainingWindow(t *testing.T) {
	t.Parallel()

	// About 49 tokens of definitions that the step's usage counts.
	tools := []fantasy.Tool{fantasy.FunctionTool{Name: "read_file", Description: strings.Repeat("d", 60)}}
	cases := []struct {
		name         string
		contextLimit int64
		inputTokens  int64
		outputTokens int64
		// trailing is the text of a message after the trigger step's
		// assistant message.
		trailing string
		// interrupted is the text of an assistant message after trailing
		// that persisted without usage.
		interrupted         string
		summaryContextLimit int64
		// omittedTools are tool definitions that the summary requests omit.
		omittedTools []fantasy.Tool
		cap          int64
		wantCap      int64
	}{
		{name: "clamps to remaining window", contextLimit: 100, inputTokens: 75, outputTokens: 10, cap: 64_000, wantCap: 13},
		{name: "reserves trailing messages", contextLimit: 100, inputTokens: 72, trailing: strings.Repeat("a", 30), cap: 64_000, wantCap: 16},
		{name: "reserves an interrupted response", contextLimit: 100, inputTokens: 72, interrupted: strings.Repeat("a", 30), cap: 64_000, wantCap: 16},
		{name: "clamps to a smaller summary window", contextLimit: 200_000, inputTokens: 150_000, summaryContextLimit: 160_000, cap: 64_000, wantCap: 9998},
		{name: "uses a larger summary window", contextLimit: 100, inputTokens: 75, outputTokens: 10, summaryContextLimit: 1_000, cap: 64_000, wantCap: 913},
		{name: "keeps cap that fits", contextLimit: 200_000, inputTokens: 140_000, outputTokens: 500, cap: 50_000, wantCap: 50_000},
		{name: "usage at limit leaves cap unchanged", contextLimit: 100, inputTokens: 100, cap: 64_000, wantCap: 64_000},
		{name: "reserves leave no room, cap unchanged", contextLimit: 100, inputTokens: 80, outputTokens: 25, cap: 64_000, wantCap: 64_000},
		// Only the omitted definitions push the estimate past the window.
		{name: "leaves omitted tool definitions out", contextLimit: 160, inputTokens: 120, summaryContextLimit: 100, omittedTools: tools, cap: 64_000, wantCap: 27},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotCap *int64
			model := &chattest.FakeModel{
				ProviderName: "fake",
				ModelName:    "fake-model",
				StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
					gotCap = call.MaxOutputTokens
					return compactionStream(
						fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
						fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "summary"},
						fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
						fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
					), nil
				},
			}
			// StepUsage measures every message up to the assistant one.
			messages := []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
				textMessage(fantasy.MessageRoleAssistant, "hi"),
			}
			if tc.trailing != "" {
				messages = append(messages, textMessage(fantasy.MessageRoleUser, tc.trailing))
			}
			unmeasuredAssistants := 0
			if tc.interrupted != "" {
				messages = append(messages, textMessage(fantasy.MessageRoleAssistant, tc.interrupted))
				unmeasuredAssistants = 1
			}
			capTokens := tc.cap
			result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
				Model:                 model,
				Messages:              messages,
				Clock:                 quartz.NewMock(t),
				ThresholdPercent:      70,
				ContextLimit:          tc.contextLimit,
				SummaryContextLimit:   tc.summaryContextLimit,
				SummaryPrompt:         "prompt",
				StepUsage:             fantasy.Usage{InputTokens: tc.inputTokens, OutputTokens: tc.outputTokens},
				SummaryTrailingTokens: EstimateTrailingTokens(messages, unmeasuredAssistants),
				SummaryCall:           fantasy.Call{MaxOutputTokens: &capTokens},
				ToolDefinitions:       tc.omittedTools,
				OmitToolDefinitions:   tc.omittedTools != nil,
			})
			require.NoError(t, err)
			require.Equal(t, "summary", result.SummaryReport)
			require.NotNil(t, gotCap)
			require.Equal(t, tc.wantCap, *gotCap)
		})
	}
}

// TestGenerateCompaction_CountsTrailingTokens verifies the threshold gate
// adds the trailing messages to the trigger step's usage while the result
// reports the measured usage.
func TestGenerateCompaction_CountsTrailingTokens(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		trailingTokens int64
		wantCalls      int
	}{
		{name: "below threshold", trailingTokens: 5},
		{name: "trailing tokens reach threshold", trailingTokens: 15, wantCalls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			model := &chattest.FakeModel{
				ProviderName: "fake",
				ModelName:    "fake-model",
				StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
					calls++
					return summaryTextStream(), nil
				},
			}
			result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
				Model:            model,
				Messages:         []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
				Clock:            quartz.NewMock(t),
				ThresholdPercent: 70,
				ContextLimit:     100,
				StepUsage:        fantasy.Usage{InputTokens: 60},
				TrailingTokens:   tc.trailingTokens,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantCalls, calls)
			if tc.wantCalls == 0 {
				require.Empty(t, result.SummaryReport)
				return
			}
			require.Equal(t, "summary", result.SummaryReport)
			require.Equal(t, int64(60), result.ContextTokens)
			require.InDelta(t, 60, result.UsagePercent, 0.001)
		})
	}
}

// TestGenerateCompaction_ElidesFirstNearTheWindow verifies that when the
// full summary request would leave too little of the window for the
// summary, the elided request goes first with a cap that fits the window,
// the full request runs only when nothing can be elided, and a rejected
// elided request ends the attempts.
func TestGenerateCompaction_ElidesFirstNearTheWindow(t *testing.T) {
	t.Parallel()

	const (
		contextLimit = int64(25_000)
		maxOutput    = int64(20_000)
	)
	toolDefinitions := []fantasy.Tool{
		fantasy.FunctionTool{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
	}
	// summarize answers every summary request with rejection, or with a
	// summary when rejection is nil.
	summarize := func(t *testing.T, messages []fantasy.Message, rejection error) ([]fantasy.Call, error) {
		t.Helper()
		var calls []fantasy.Call
		model := &chattest.FakeModel{
			ProviderName: "fake",
			ModelName:    "fake-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				calls = append(calls, call)
				if rejection != nil {
					return nil, rejection
				}
				return summaryTextStream(), nil
			},
		}
		capTokens := maxOutput
		result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
			Model:            model,
			Messages:         messages,
			Clock:            quartz.NewMock(t),
			ThresholdPercent: 70,
			ContextLimit:     contextLimit,
			SummaryPrompt:    "summarize",
			// The trigger step's 20,100 tokens, the trailing tool result,
			// and the summary prompt leave under 16,000 tokens of the
			// window for the summary.
			StepUsage:             fantasy.Usage{InputTokens: 20_000, OutputTokens: 100},
			SummaryTrailingTokens: EstimateTrailingTokens(messages, 0),
			SummaryCall:           fantasy.Call{MaxOutputTokens: &capTokens},
			ToolDefinitions:       toolDefinitions,
		})
		if err == nil {
			require.Equal(t, "summary", result.SummaryReport)
		}
		return calls, err
	}

	t.Run("elides before the full request", func(t *testing.T) {
		t.Parallel()

		calls, err := summarize(t, toolResultHistory(strings.Repeat("a", 60_000)), nil)
		require.NoError(t, err)
		require.Len(t, calls, 1)
		require.Nil(t, calls[0].Tools)
		require.Contains(t, toolResultTexts(calls[0].Prompt)[0], "Coder truncated")
		require.NotNil(t, calls[0].MaxOutputTokens)
		require.Less(t, *calls[0].MaxOutputTokens, maxOutput)
		require.LessOrEqual(t, estimateMessagesTokens(calls[0].Prompt)+*calls[0].MaxOutputTokens, contextLimit)
	})

	t.Run("sends the full request when nothing can be elided", func(t *testing.T) {
		t.Parallel()

		calls, err := summarize(t, toolResultHistory("small result"), nil)
		require.NoError(t, err)
		require.Len(t, calls, 1)
		require.Equal(t, toolDefinitions, calls[0].Tools)
		require.Equal(t, []string{"small result"}, toolResultTexts(calls[0].Prompt))
		require.NotNil(t, calls[0].MaxOutputTokens)
		// 25,000 minus 20,100 for the step, 4 for the trailing result,
		// and 3 for the summary prompt.
		require.Equal(t, int64(4_893), *calls[0].MaxOutputTokens)
	})

	t.Run("stops when the elided request is rejected", func(t *testing.T) {
		t.Parallel()

		calls, err := summarize(t, toolResultHistory(strings.Repeat("a", 60_000)), contextWindowError())
		require.Error(t, err)
		require.Equal(t, codersdk.ChatErrorKindConfig, chaterror.Classify(err).Kind)
		require.Len(t, calls, 1, "the full requests are larger than the rejected elided one")
		require.Nil(t, calls[0].Tools)
	})
}

// TestGenerateCompaction_ElideFirstRoomFloor pins when the elided request
// goes first: the full request must leave less room in a known window than
// the smaller of 16,000 tokens and the output cap.
func TestGenerateCompaction_ElideFirstRoomFloor(t *testing.T) {
	t.Parallel()

	// The step's 50,000 tokens, 2,000 for the trailing tool result, and 3
	// for the summary prompt.
	const fullInput = int64(52_003)
	readFile := []fantasy.Tool{fantasy.FunctionTool{Name: "read_file"}}
	// About 10,000 tokens of definitions that the step's usage counts.
	largeTools := []fantasy.Tool{fantasy.FunctionTool{Name: "read_file", Description: strings.Repeat("d", 30_000)}}
	cases := []struct {
		name      string
		window    int64
		maxOutput int64
		// usage defaults to the step's 50,000 input tokens.
		usage      fantasy.Usage
		tools      []fantasy.Tool
		omitTools  bool
		wantElided bool
	}{
		{name: "room below the floor", window: fullInput + 15_999, maxOutput: 20_000, tools: readFile, wantElided: true},
		{name: "room at the floor", window: fullInput + 16_000, maxOutput: 20_000, tools: readFile},
		{name: "a smaller output cap lowers the floor", window: fullInput + 10_000, maxOutput: 4_000, tools: readFile},
		{name: "unknown window", maxOutput: 20_000, tools: readFile},
		{name: "omitted tool definitions leave room", window: fullInput + 10_000, maxOutput: 20_000, tools: largeTools, omitTools: true},
		// The total already counts the step's output.
		{name: "a total-only usage counts the output once", window: fullInput + 16_000, maxOutput: 20_000, usage: fantasy.Usage{TotalTokens: 50_000, OutputTokens: 4_000}, tools: readFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls []fantasy.Call
			model := &chattest.FakeModel{
				ProviderName: "fake",
				ModelName:    "fake-model",
				StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
					calls = append(calls, call)
					return summaryTextStream(), nil
				},
			}
			messages := toolResultHistory(strings.Repeat("a", 6_000))
			_, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
				Model:                 model,
				Messages:              messages,
				SummaryTrailingTokens: EstimateTrailingTokens(messages, 0),
				Clock:                 quartz.NewMock(t),
				ContextLimit:          tc.window,
				SummaryPrompt:         "summarize",
				StepUsage:             cmp.Or(tc.usage, fantasy.Usage{InputTokens: 50_000}),
				SummaryCall:           fantasy.Call{MaxOutputTokens: ptr.Ref(tc.maxOutput)},
				ToolDefinitions:       tc.tools,
				OmitToolDefinitions:   tc.omitTools,
				Force:                 true,
			})
			require.NoError(t, err)
			require.Len(t, calls, 1)
			texts := toolResultTexts(calls[0].Prompt)
			require.Len(t, texts, 1)
			if tc.wantElided {
				require.Contains(t, texts[0], "Coder truncated")
				require.Nil(t, calls[0].Tools)
				return
			}
			require.Equal(t, strings.Repeat("a", 6_000), texts[0])
			if tc.omitTools {
				require.Nil(t, calls[0].Tools)
				return
			}
			require.Equal(t, tc.tools, calls[0].Tools)
		})
	}
}

func TestStepOutputTokens(t *testing.T) {
	t.Parallel()

	require.EqualValues(t, 500, StepOutputTokens(fantasy.Usage{InputTokens: 1_000, CacheReadTokens: 2_000, OutputTokens: 500}))
	// The total already counts the output.
	require.Zero(t, StepOutputTokens(fantasy.Usage{TotalTokens: 1_500, OutputTokens: 500}))
}

func TestElideToolResults(t *testing.T) {
	t.Parallel()

	toolResult := func(callID, text string) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{ToolCallID: callID, Output: fantasy.ToolResultOutputContentText{Text: text}}
	}
	toolErr := xerrors.New("boom")
	newMessages := func() []fantasy.Message {
		return []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
			{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.ToolResultPart{
					ToolCallID:       "web_search",
					ProviderExecuted: true,
					Output:           fantasy.ToolResultOutputContentText{Text: strings.Repeat("p", 50_000)},
				}},
			},
			{
				Role: fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{
					toolResult("a", strings.Repeat("a", 30_000)),
					toolResult("b", strings.Repeat("b", 20_000)),
					toolResult("c", strings.Repeat("c", 4_000)),
					toolResult("d", strings.Repeat("d", 500)),
					fantasy.ToolResultPart{ToolCallID: "e", Output: fantasy.ToolResultOutputContentError{Error: toolErr}},
				},
			},
		}
	}

	t.Run("cuts the largest results to a common size", func(t *testing.T) {
		t.Parallel()

		messages := newMessages()
		// Cutting about 26,000 bytes takes the two largest results to about
		// 12,000 bytes each, above the third largest.
		budget := estimateMessagesTokens(messages) - 26_000/bytesPerTokenEstimate
		elided, removed := elideToolResults(messages, budget)
		require.Equal(t, newMessages(), messages, "the caller's messages must not change")
		require.LessOrEqual(t, estimateMessagesTokens(elided), budget)

		texts := toolResultTexts(elided)
		require.Len(t, texts, 5)
		require.Equal(t, strings.Repeat("p", 50_000), texts[0], "provider-executed results are not cut")
		require.Contains(t, texts[1], "Coder truncated")
		require.Contains(t, texts[2], "Coder truncated")
		require.InDelta(t, len(texts[1]), len(texts[2]), 16)
		require.Greater(t, len(texts[2]), 4_000)
		require.Equal(t, strings.Repeat("c", 4_000), texts[3])
		require.Equal(t, strings.Repeat("d", 500), texts[4])
		require.Equal(t, messages[2].Content[4], elided[2].Content[4])
		require.Equal(t, 50_000-len(texts[1])-len(texts[2]), removed)
	})

	t.Run("keeps a floor of each result", func(t *testing.T) {
		t.Parallel()

		elided, removed := elideToolResults(newMessages(), 0)
		require.Positive(t, removed)
		texts := toolResultTexts(elided)
		require.Len(t, texts, 5)
		require.Len(t, texts[0], 50_000)
		for _, text := range texts[1:4] {
			require.Contains(t, text, "Coder truncated")
			require.LessOrEqual(t, len(text), elidedToolResultMinBytes)
		}
		require.Equal(t, strings.Repeat("d", 500), texts[4])
	})

	t.Run("leaves a history that fits", func(t *testing.T) {
		t.Parallel()

		messages := newMessages()
		elided, removed := elideToolResults(messages, estimateMessagesTokens(messages))
		require.Zero(t, removed)
		require.Equal(t, messages, elided)
	})

	t.Run("cuts text attachments and keeps media", func(t *testing.T) {
		t.Parallel()

		image := fantasy.FilePart{Filename: "shot.png", Data: make([]byte, 30_000), MediaType: "image/png"}
		messages := []fantasy.Message{{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.FilePart{Filename: "notes.txt", Data: []byte(strings.Repeat("n", 30_000)), MediaType: "text/plain; charset=utf-8"},
				image,
			},
		}}
		elided, removed := elideToolResults(messages, 0)
		require.Positive(t, removed)
		file, ok := fantasy.AsMessagePart[fantasy.FilePart](elided[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "notes.txt", file.Filename)
		require.Equal(t, "text/plain; charset=utf-8", file.MediaType)
		require.Contains(t, string(file.Data), "Coder truncated")
		require.LessOrEqual(t, len(file.Data), elidedToolResultMinBytes)
		require.Equal(t, 30_000-len(file.Data), removed)
		require.Equal(t, image, elided[0].Content[1])
	})

	t.Run("cuts user text and keeps system and assistant text", func(t *testing.T) {
		t.Parallel()

		system := textMessage(fantasy.MessageRoleSystem, strings.Repeat("s", 30_000))
		assistant := textMessage(fantasy.MessageRoleAssistant, strings.Repeat("r", 30_000))
		// Providers that reject text attachments get them inlined as
		// user text.
		messages := []fantasy.Message{system, textMessage(fantasy.MessageRoleUser, strings.Repeat("u", 30_000)), assistant}
		elided, removed := elideToolResults(messages, 0)
		require.Positive(t, removed)
		require.Equal(t, system, elided[0])
		require.Equal(t, assistant, elided[2])
		text, ok := fantasy.AsMessagePart[fantasy.TextPart](elided[1].Content[0])
		require.True(t, ok)
		require.Contains(t, text.Text, "Coder truncated")
		require.Equal(t, 30_000-len(text.Text), removed)
	})

	t.Run("cuts error results and keeps them errors", func(t *testing.T) {
		t.Parallel()

		messages := []fantasy.Message{{
			Role: fantasy.MessageRoleTool,
			Content: []fantasy.MessagePart{fantasy.ToolResultPart{
				ToolCallID: "a",
				Output:     fantasy.ToolResultOutputContentError{Error: xerrors.New(strings.Repeat("e", 30_000))},
			}},
		}}
		elided, removed := elideToolResults(messages, 0)
		require.Positive(t, removed)
		result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](elided[0].Content[0])
		require.True(t, ok)
		output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output)
		require.True(t, ok, "an elided error result stays an error")
		require.Contains(t, output.Error.Error(), "Coder truncated")
		require.LessOrEqual(t, len(output.Error.Error()), elidedToolResultMinBytes)
	})
}

func TestEstimateTrailingTokens(t *testing.T) {
	t.Parallel()

	toolCall := fantasy.Message{
		Role:    fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: "call", ToolName: "read_file", Input: "{}"}},
	}
	toolResult := fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{fantasy.ToolResultPart{
			ToolCallID: "call",
			Output:     fantasy.ToolResultOutputContentText{Text: strings.Repeat("x", 300)},
		}},
	}
	media := fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.FilePart{Data: make([]byte, 1<<20), MediaType: "image/png"},
			fantasy.FilePart{Data: make([]byte, 1<<20), MediaType: "application/pdf"},
		},
	}
	document := fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.FilePart{Data: make([]byte, 300), MediaType: "text/plain; charset=utf-8"}},
	}
	cases := []struct {
		name                 string
		messages             []fantasy.Message
		unmeasuredAssistants int
		want                 int64
	}{
		{
			name:     "ends with an assistant message",
			messages: []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello"), toolCall},
			want:     0,
		},
		{
			name: "counts every message after the last assistant message",
			messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
				toolCall,
				toolResult,
				textMessage(fantasy.MessageRoleUser, "abc"),
			},
			want: 101,
		},
		{
			name: "counts unmeasured assistant messages",
			messages: []fantasy.Message{
				toolCall,
				toolResult,
				textMessage(fantasy.MessageRoleAssistant, "abc"),
			},
			unmeasuredAssistants: 1,
			want:                 101,
		},
		{
			name:     "counts images and PDFs by part, not by size",
			messages: []fantasy.Message{toolCall, media},
			want:     2 * mediaTokenEstimate,
		},
		{
			name:     "counts a text document by size",
			messages: []fantasy.Message{toolCall, document},
			want:     100,
		},
		{
			name:     "counts the whole history without an assistant message",
			messages: []fantasy.Message{textMessage(fantasy.MessageRoleUser, "abc")},
			want:     1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, EstimateTrailingTokens(tc.messages, tc.unmeasuredAssistants))
		})
	}
}

// TestGenerateCompaction_DefaultSourceAutomatic verifies an unforced
// over-threshold run reports the automatic source by default.
func TestGenerateCompaction_DefaultSourceAutomatic(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return compactionStream(
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "auto summary"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
				fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			), nil
		},
	}
	result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
		Model:            model,
		Messages:         []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		ThresholdPercent: 70,
		ContextLimit:     100,
		StepUsage:        fantasy.Usage{InputTokens: 90},
		Clock:            quartz.NewMock(t),
	})
	require.NoError(t, err)
	require.Equal(t, "auto summary", result.SummaryReport)
	require.Equal(t, CompactionSourceAutomatic, result.Source)
}

func TestGenerateCompaction_SummaryEstimate(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"P", "Pr", "Pre"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			var parts []codersdk.ChatMessagePart
			result, err := GenerateCompaction(t.Context(), GenerateCompactionOptions{
				Model: &chattest.FakeModel{
					StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
						return compactionStream(
							fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
							fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "界x"},
							fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
							fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
						), nil
					},
				},
				Messages:            []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
				SystemSummaryPrefix: prefix,
				Force:               true,
				ContextLimit:        1000,
				StepUsage:           fantasy.Usage{InputTokens: 800},
				ToolCallID:          "summary",
				ToolName:            "chat_summarized",
				PublishMessagePart: func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
					parts = append(parts, part)
				},
				Clock: quartz.NewMock(t),
			})
			require.NoError(t, err)
			require.Equal(t, prefix+"\n\n界x", result.SystemSummary)
			require.Equal(t, int64(3), result.EstimatedContextTokens)
			require.Equal(t, int64(800), result.ContextTokens)
			require.Len(t, parts, 2)
			require.Equal(t, codersdk.ChatMessagePartTypeToolResult, parts[1].Type)
			require.False(t, parts[1].IsError)
			var metadata map[string]any
			require.NoError(t, json.Unmarshal(parts[1].Result, &metadata))
			require.Equal(t, float64(3), metadata["estimated_context_tokens"])
			require.Equal(t, float64(1000), metadata["trigger_context_limit_tokens"])
			require.Equal(t, float64(0), metadata["context_limit_tokens"])
		})
	}
}

// TestGenerateCompaction_RecordsRequestedTrigger verifies the result echoes
// the requested trigger, not the normalized one, while UsagePercent uses the
// resolved limit.
func TestGenerateCompaction_RecordsRequestedTrigger(t *testing.T) {
	t.Parallel()

	var parts []codersdk.ChatMessagePart
	result, err := GenerateCompaction(t.Context(), GenerateCompactionOptions{
		Model: &chattest.FakeModel{
			StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				return compactionStream(
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: "summary"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
					fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				), nil
			},
		},
		Messages:             []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		Force:                true,
		ThresholdPercent:     101,
		ContextLimit:         0,
		ContextLimitFallback: 1000,
		ChatContextLimit:     2000,
		StepUsage:            fantasy.Usage{InputTokens: 800},
		ToolCallID:           "summary",
		ToolName:             "chat_summarized",
		PublishMessagePart: func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			parts = append(parts, part)
		},
		Clock: quartz.NewMock(t),
	})
	require.NoError(t, err)
	require.Equal(t, int32(101), result.ThresholdPercent)
	require.Zero(t, result.TriggerContextLimit)
	require.Equal(t, int64(2000), result.ChatContextLimit)
	require.InDelta(t, 80, result.UsagePercent, 0.001)

	require.Len(t, parts, 2)
	var streamed CompactionToolResult
	require.NoError(t, json.Unmarshal(parts[1].Result, &streamed))
	require.Equal(t, int32(101), streamed.ThresholdPercent)
	require.Zero(t, streamed.TriggerContextLimitTokens)
	require.Equal(t, int64(2000), streamed.ContextLimitTokens)
}

// TestGenerateCompaction_RequiresClock verifies a nil clock is
// rejected instead of silently falling back to a real clock; tests
// must supply their own.
func TestGenerateCompaction_RequiresClock(t *testing.T) {
	t.Parallel()

	_, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
		Model: &chattest.FakeModel{ProviderName: "fake", ModelName: "fake-model"},
	})
	require.ErrorContains(t, err, "clock is required")
}
