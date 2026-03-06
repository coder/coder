package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

func TestRun_Compaction(t *testing.T) {
	t.Parallel()

	t.Run("PersistsWhenThresholdReached", func(t *testing.T) {
		t.Parallel()

		persistCompactionCalls := 0
		var persistedCompaction CompactionResult
		const summaryText = "summary text for compaction"

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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
				PublishMessagePart: func(role fantasy.MessageRole, part codersdk.ChatMessagePart) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
				PublishMessagePart: func(_ fantasy.MessageRole, _ codersdk.ChatMessagePart) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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

		model := &loopTestModel{
			provider: "fake",
			streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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
}
