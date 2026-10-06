package chatloop

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
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
			name: "no tools to drop",
			err:  contextTooLarge,
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

			_, _, err := generateCompactionSummary(context.Background(), model,
				[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
				CompactionOptions{
					SummaryPrompt:   "summarize",
					ToolDefinitions: tc.toolDefinitions,
				},
			)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, 1, calls)
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
// shrinks to the remaining window minus the trigger step's output and
// the appended summary prompt (ceil(bytes/3) tokens), and degenerate
// cases stay unchanged.
func TestGenerateCompaction_ClampsSummaryCapToRemainingWindow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		contextLimit    int64
		inputTokens     int64
		outputTokens    int64
		toolResultBytes int
		cap             int64
		wantCap         int64
	}{
		{name: "clamps to remaining window", contextLimit: 100, inputTokens: 75, outputTokens: 10, cap: 64_000, wantCap: 13},
		{name: "reserves trailing tool results", contextLimit: 100, inputTokens: 72, toolResultBytes: 30, cap: 64_000, wantCap: 16},
		{name: "keeps cap that fits", contextLimit: 200_000, inputTokens: 140_000, outputTokens: 500, cap: 50_000, wantCap: 50_000},
		{name: "usage at limit leaves cap unchanged", contextLimit: 100, inputTokens: 100, cap: 64_000, wantCap: 64_000},
		{name: "reserves leave no room, cap unchanged", contextLimit: 100, inputTokens: 80, outputTokens: 25, cap: 64_000, wantCap: 64_000},
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
			capTokens := tc.cap
			messages := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}
			if tc.toolResultBytes > 0 {
				messages = append(messages, fantasy.Message{
					Role: fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{fantasy.ToolResultPart{
						ToolCallID: "call-1",
						Output:     fantasy.ToolResultOutputContentText{Text: strings.Repeat("x", tc.toolResultBytes)},
					}},
				})
			}
			result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
				Model:            model,
				Messages:         messages,
				Clock:            quartz.NewMock(t),
				ThresholdPercent: 70,
				ContextLimit:     tc.contextLimit,
				SummaryPrompt:    "prompt",
				StepUsage:        fantasy.Usage{InputTokens: tc.inputTokens, OutputTokens: tc.outputTokens},
				SummaryCall:      fantasy.Call{MaxOutputTokens: &capTokens},
			})
			require.NoError(t, err)
			require.Equal(t, "summary", result.SummaryReport)
			require.NotNil(t, gotCap)
			require.Equal(t, tc.wantCap, *gotCap)
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
			require.Equal(t, float64(1000), metadata["context_limit_tokens"])
		})
	}
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
