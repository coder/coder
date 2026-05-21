package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

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
			Kind:                chatdebug.KindChatTurn,
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
		reportedErr := make(chan error, 1)

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{}, xerrors.New("insert compaction debug run"))

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
			OnError: func(err error) {
				reportedErr <- err
			},
		})
		require.Same(t, ctx, compactionCtx)
		finish(nil)
		select {
		case err := <-reportedErr:
			t.Fatalf("unexpected OnError callback: %v", err)
		default:
		}
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
		reportedErr := make(chan error, 1)
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
			OnError: func(err error) {
				reportedErr <- err
			},
		})
		require.NotSame(t, ctx, compactionCtx)
		finish(nil)
		select {
		case err := <-reportedErr:
			t.Fatalf("unexpected OnError callback: %v", err)
		default:
		}
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
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			panic("compaction model crash")
		},
	}

	parentCtx := chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
		RunID:               uuid.New(),
		ChatID:              chatID,
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    1,
		HistoryTipMessageID: 2,
		Kind:                chatdebug.KindChatTurn,
		Provider:            "fake",
		Model:               "fake-model",
	})

	require.PanicsWithValue(t, "compaction model crash", func() {
		_, _ = generateCompactionSummary(parentCtx, model,
			[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
			CompactionOptions{
				DebugSvc:      svc,
				ChatID:        chatID,
				SummaryPrompt: "summarize",
				Timeout:       time.Second,
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

func TestRun_Compaction(t *testing.T) {
	t.Parallel()

	t.Run("PersistsWhenThresholdReached", func(t *testing.T) {
		t.Parallel()

		persistCompactionCalls := 0
		var persistedCompaction CompactionResult
		const summaryText = "summary text for compaction"

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonStop,
						Usage: fantasy.Usage{
							InputTokens: 80,
							TotalTokens: 85,
						},
					},
				}), nil
			},
			GenerateFn: func(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
				require.NotEmpty(t, call.Prompt)
				lastPrompt := call.Prompt[len(call.Prompt)-1]
				require.Equal(t, fantasy.MessageRoleUser, lastPrompt.Role)
				require.Len(t, lastPrompt.Content, 1)

				instruction, ok := fantasy.AsMessagePart[fantasy.TextPart](lastPrompt.Content[0])
				require.True(t, ok)
				require.Equal(t, "summarize now", instruction.Text)

				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, result CompactionResult) error {
					persistCompactionCalls++
					persistedCompaction = result
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "hello"),
				}, nil
			},
		})
		require.NoError(t, err)
		// Compaction fires twice: once inline when the threshold is
		// reached on step 0 (the only step, since MaxSteps=1), and
		// once from the post-run safety net during the re-entry
		// iteration (where totalSteps already equals MaxSteps so the
		// inner loop doesn't execute, but lastUsage still exceeds
		// the threshold).
		require.Equal(t, 2, persistCompactionCalls)
		require.Contains(t, persistedCompaction.SystemSummary, summaryText)
		require.Equal(t, summaryText, persistedCompaction.SummaryReport)
		require.Equal(t, int64(80), persistedCompaction.ContextTokens)
		require.Equal(t, int64(100), persistedCompaction.ContextLimit)
		require.InDelta(t, 80.0, persistedCompaction.UsagePercent, 0.0001)
	})

	t.Run("PublishesPartsBeforeAndAfterPersist", func(t *testing.T) {
		t.Parallel()

		const summaryText = "compaction summary for ordering test"

		// Track the order of callbacks to verify the tool-call
		// part publishes before Generate (summary generation)
		// and the tool-result part publishes after Persist.
		var callOrder []string

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonStop,
						Usage: fantasy.Usage{
							InputTokens: 80,
							TotalTokens: 85,
						},
					},
				}), nil
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				callOrder = append(callOrder, "generate")
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				ToolCallID:       "test-tool-call-id",
				ToolName:         "chat_summarized",
				PublishMessagePart: func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
					switch part.Type {
					case codersdk.ChatMessagePartTypeToolCall:
						callOrder = append(callOrder, "publish_tool_call")
					case codersdk.ChatMessagePartTypeToolResult:
						callOrder = append(callOrder, "publish_tool_result")
					}
				},
				Persist: func(_ context.Context, _ CompactionResult) error {
					callOrder = append(callOrder, "persist")
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "hello"),
				}, nil
			},
		})
		require.NoError(t, err)
		// Compaction fires twice (see PersistsWhenThresholdReached
		// for the full explanation). Each cycle follows the order:
		// publish_tool_call → generate → persist → publish_tool_result.
		require.Equal(t, []string{
			"publish_tool_call",
			"generate",
			"persist",
			"publish_tool_result",
			"publish_tool_call",
			"generate",
			"persist",
			"publish_tool_result",
		}, callOrder)
	})

	t.Run("PublishNotCalledBelowThreshold", func(t *testing.T) {
		t.Parallel()

		publishCalled := false

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonStop,
						Usage: fantasy.Usage{
							InputTokens: 10,
						},
					},
				}), nil
			},
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				ToolCallID:       "test-tool-call-id",
				ToolName:         "chat_summarized",
				PublishMessagePart: func(_ codersdk.ChatMessageRole, _ codersdk.ChatMessagePart) {
					publishCalled = true
				},
				Persist: func(_ context.Context, _ CompactionResult) error {
					return nil
				},
			},
		})
		require.NoError(t, err)
		require.False(t, publishCalled, "PublishMessagePart should not fire when usage is below threshold")
	})

	t.Run("MidLoopCompactionReloadsMessages", func(t *testing.T) {
		t.Parallel()

		var mu sync.Mutex
		var streamCallCount int
		persistCompactionCalls := 0
		reloadCalls := 0

		const summaryText = "compacted summary"

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				mu.Lock()
				step := streamCallCount
				streamCallCount++
				mu.Unlock()

				switch step {
				case 0:
					// Step 0: tool call with high usage (80/100 = 80% > 70%).
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "read_file"},
						{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{}`},
						{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
						{
							Type:          fantasy.StreamPartTypeToolCall,
							ID:            "tc-1",
							ToolCallName:  "read_file",
							ToolCallInput: `{}`,
						},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonToolCalls,
							Usage: fantasy.Usage{
								InputTokens: 80,
								TotalTokens: 85,
							},
						},
					}), nil
				default:
					// Step 1: text with low usage (30/100 = 30% < 70%).
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 30,
								TotalTokens: 35,
							},
						},
					}), nil
				}
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		compactedMessages := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "compacted system"),
			textMessage(fantasy.MessageRoleUser, "compacted user"),
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			Tools: []fantasy.AgentTool{
				newNoopTool("read_file"),
			},
			MaxSteps: 5,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, _ CompactionResult) error {
					persistCompactionCalls++
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				reloadCalls++
				return compactedMessages, nil
			},
		})
		require.NoError(t, err)

		// Compaction fired after step 0 (above threshold).
		require.GreaterOrEqual(t, persistCompactionCalls, 1)
		// ReloadMessages was called after mid-loop compaction.
		require.GreaterOrEqual(t, reloadCalls, 1)
		// Both steps ran (tool-call step + follow-up text step).
		require.Equal(t, 2, streamCallCount)
	})

	t.Run("PostRunCompactionSkippedAfterMidLoop", func(t *testing.T) {
		t.Parallel()

		var mu sync.Mutex
		var streamCallCount int
		persistCompactionCalls := 0

		const summaryText = "compacted summary for skip test"

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				mu.Lock()
				step := streamCallCount
				streamCallCount++
				mu.Unlock()

				switch step {
				case 0:
					// Step 0: tool call with high usage (80/100 = 80% > 70%).
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "read_file"},
						{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{}`},
						{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
						{
							Type:          fantasy.StreamPartTypeToolCall,
							ID:            "tc-1",
							ToolCallName:  "read_file",
							ToolCallInput: `{}`,
						},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonToolCalls,
							Usage: fantasy.Usage{
								InputTokens: 80,
								TotalTokens: 85,
							},
						},
					}), nil
				default:
					// Step 1: text with low usage (20/100 = 20% < 70%).
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 20,
								TotalTokens: 25,
							},
						},
					}), nil
				}
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		compactedMessages := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "compacted system"),
			textMessage(fantasy.MessageRoleUser, "compacted user"),
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			Tools: []fantasy.AgentTool{
				newNoopTool("read_file"),
			},
			MaxSteps: 5,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, _ CompactionResult) error {
					persistCompactionCalls++
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return compactedMessages, nil
			},
		})
		require.NoError(t, err)

		// Only mid-loop compaction fires after step 0. The post-run
		// safety net is skipped because alreadyCompacted is true.
		require.Equal(t, 1, persistCompactionCalls)
	})

	t.Run("ErrorsAreReported", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonStop,
						Usage: fantasy.Usage{
							InputTokens: 80,
						},
					},
				}), nil
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return nil, xerrors.New("generate failed")
			},
		}

		compactionErr := xerrors.New("unset")
		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				Persist: func(_ context.Context, _ CompactionResult) error {
					return nil
				},
				OnError: func(err error) {
					compactionErr = err
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "hello"),
				}, nil
			},
		})
		require.NoError(t, err)
		require.Error(t, compactionErr)
		require.ErrorContains(t, compactionErr, "generate summary text")
	})

	t.Run("PostRunCompactionReEntersStepLoop", func(t *testing.T) {
		t.Parallel()

		// When post-run compaction fires (no mid-loop compaction)
		// and ReloadMessages is provided, Run should re-enter the
		// step loop with the reloaded messages so the agent
		// continues working.

		var mu sync.Mutex
		var streamCallCount int
		persistCompactionCalls := 0
		reloadCalls := 0

		const summaryText = "post-run compacted summary"

		compactedMessages := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "compacted system"),
			textMessage(fantasy.MessageRoleUser, "compacted user"),
		}

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				mu.Lock()
				step := streamCallCount
				streamCallCount++
				mu.Unlock()

				switch step {
				case 0:
					// First turn: text-only response with high usage.
					// No tool calls, so shouldContinue = false and
					// the inner step loop breaks. Compaction should
					// fire, then the outer loop re-enters.
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "initial response"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 80,
								TotalTokens: 85,
							},
						},
					}), nil
				default:
					// Second turn (after compaction re-entry):
					// text-only with low usage — should finish.
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-2"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-2", Delta: "continued after compaction"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-2"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 20,
								TotalTokens: 25,
							},
						},
					}), nil
				}
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 5,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, _ CompactionResult) error {
					persistCompactionCalls++
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				reloadCalls++
				return compactedMessages, nil
			},
		})
		require.NoError(t, err)

		// Compaction fired on the final step of the first pass.
		// The inline path fires (ReloadMessages is set) and then
		// the outer loop re-enters. On the second pass the usage
		// is below threshold so no further compaction occurs.
		require.GreaterOrEqual(t, persistCompactionCalls, 1)
		// ReloadMessages was called (inline + re-entry).
		require.GreaterOrEqual(t, reloadCalls, 1)
		// Two stream calls: one before compaction, one after re-entry.
		require.Equal(t, 2, streamCallCount)
	})

	t.Run("PostRunCompactionReEntryIncludesUserSummary", func(t *testing.T) {
		t.Parallel()

		// After compaction the summary is stored as a user-role
		// message. When the loop re-enters, the reloaded prompt
		// must contain this user message so the LLM provider
		// receives a valid prompt (providers like Anthropic
		// require at least one non-system message).

		var mu sync.Mutex
		var streamCallCount int
		var reEntryPrompt []fantasy.Message
		persistCompactionCalls := 0

		const summaryText = "post-run compacted summary"

		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				mu.Lock()
				step := streamCallCount
				streamCallCount++
				mu.Unlock()

				switch step {
				case 0:
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "initial response"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 80,
								TotalTokens: 85,
							},
						},
					}), nil
				default:
					mu.Lock()
					reEntryPrompt = append([]fantasy.Message(nil), call.Prompt...)
					mu.Unlock()
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-2"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-2", Delta: "continued"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-2"},
						{
							Type:         fantasy.StreamPartTypeFinish,
							FinishReason: fantasy.FinishReasonStop,
							Usage: fantasy.Usage{
								InputTokens: 20,
								TotalTokens: 25,
							},
						},
					}), nil
				}
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		// Simulate real post-compaction DB state: the summary is
		// a user-role message (the only non-system content).
		compactedMessages := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "system prompt"),
			textMessage(fantasy.MessageRoleUser, "Summary of earlier chat context:\n\ncompacted summary"),
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps: 5,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, _ CompactionResult) error {
					persistCompactionCalls++
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return compactedMessages, nil
			},
		})
		require.NoError(t, err)

		require.GreaterOrEqual(t, persistCompactionCalls, 1)
		// Re-entry happened: stream was called at least twice.
		require.Equal(t, 2, streamCallCount)
		// The re-entry prompt must contain the user summary.
		require.NotEmpty(t, reEntryPrompt)
		hasUser := false
		for _, msg := range reEntryPrompt {
			if msg.Role == fantasy.MessageRoleUser {
				hasUser = true
				break
			}
		}
		require.True(t, hasUser, "re-entry prompt must contain a user message (the compaction summary)")
	})

	t.Run("TriggersOnDynamicToolExit", func(t *testing.T) {
		t.Parallel()

		var persistCompactionCalls int
		const summaryText = "compaction summary for dynamic tool exit"

		// The LLM calls a dynamic tool. Usage is above the
		// compaction threshold so compaction should fire even
		// though the chatloop exits via ErrDynamicToolCall.
		model := &chattest.FakeModel{
			ProviderName: "fake",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "my_dynamic_tool"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{"query": "test"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-1",
						ToolCallName:  "my_dynamic_tool",
						ToolCallInput: `{"query": "test"}`,
					},
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonToolCalls,
						Usage: fantasy.Usage{
							InputTokens: 80,
							TotalTokens: 85,
						},
					},
				}), nil
			},
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: summaryText},
					},
				}, nil
			},
		}

		err := Run(context.Background(), RunOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
			MaxSteps:         5,
			DynamicToolNames: map[string]bool{"my_dynamic_tool": true},
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			ContextLimitFallback: 100,
			Compaction: &CompactionOptions{
				ThresholdPercent: 70,
				SummaryPrompt:    "summarize now",
				Persist: func(_ context.Context, result CompactionResult) error {
					persistCompactionCalls++
					require.Contains(t, result.SystemSummary, summaryText)
					return nil
				},
			},
			ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
				return []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "hello"),
				}, nil
			},
		})
		require.ErrorIs(t, err, ErrDynamicToolCall)
		require.Equal(t, 1, persistCompactionCalls,
			"compaction must fire before dynamic tool exit")
	})
}
