package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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

		_, err := Run(context.Background(), RunOptions{
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
		})
		require.NoError(t, err)
		require.Equal(t, 1, persistCompactionCalls)
		require.Contains(t, persistedCompaction.SystemSummary, summaryText)
		require.Equal(t, summaryText, persistedCompaction.SummaryReport)
		require.Equal(t, int64(80), persistedCompaction.ContextTokens)
		require.Equal(t, int64(100), persistedCompaction.ContextLimit)
		require.InDelta(t, 80.0, persistedCompaction.UsagePercent, 0.0001)
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
		_, err := Run(context.Background(), RunOptions{
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
		})
		require.NoError(t, err)
		require.Error(t, compactionErr)
		require.ErrorContains(t, compactionErr, "generate summary text")
	})
}
