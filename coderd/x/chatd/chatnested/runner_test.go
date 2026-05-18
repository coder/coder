package chatnested_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatnested"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestRunTextStreamsDeltasAndReturnsFinalText(t *testing.T) {
	t.Parallel()

	var deltas []string
	result, err := chatnested.RunText(t.Context(), chatnested.RunTextOptions{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				require.Empty(t, call.Tools)
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hello "},
					{Type: fantasy.StreamPartTypeReasoningStart, ID: "reasoning-1"},
					{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning-1", Delta: "hidden reasoning"},
					{Type: fantasy.StreamPartTypeReasoningEnd, ID: "reasoning-1"},
					{Type: fantasy.StreamPartTypeSource, ID: "source-1", URL: "https://example.test"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "world"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: fantasy.Usage{
						InputTokens:         12,
						OutputTokens:        5,
						TotalTokens:         17,
						ReasoningTokens:     2,
						CacheCreationTokens: 3,
						CacheReadTokens:     4,
					}},
				}), nil
			},
		},
		Messages:             []fantasy.Message{textMessage("question?")},
		ContextLimitFallback: 128000,
		OnTextDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"hello ", "world"}, deltas)
	require.Equal(t, "hello world", result.Text)
	require.EqualValues(t, 12, ptrValue(result.Usage.InputTokens))
	require.EqualValues(t, 5, ptrValue(result.Usage.OutputTokens))
	require.EqualValues(t, 17, ptrValue(result.Usage.TotalTokens))
	require.EqualValues(t, 2, ptrValue(result.Usage.ReasoningTokens))
	require.EqualValues(t, 3, ptrValue(result.Usage.CacheCreationTokens))
	require.EqualValues(t, 4, ptrValue(result.Usage.CacheReadTokens))
	require.True(t, result.ContextLimit.Valid)
	require.EqualValues(t, 128000, result.ContextLimit.Int64)
	require.GreaterOrEqual(t, result.Runtime, time.Duration(0))
}

func TestRunTextResetsDeltasOnRetry(t *testing.T) {
	t.Parallel()

	var (
		calls  int
		events []string
	)
	result, err := chatnested.RunText(t.Context(), chatnested.RunTextOptions{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				calls++
				if calls == 1 {
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "stale "},
						{Type: fantasy.StreamPartTypeError, Error: xerrors.New("received status 429 from upstream")},
					}), nil
				}
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "fresh"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		Messages: []fantasy.Message{textMessage("question?")},
		OnTextDelta: func(delta string) {
			events = append(events, "delta:"+delta)
		},
		OnTextReset: func() {
			events = append(events, "reset")
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"delta:stale ", "reset", "delta:fresh"}, events)
	require.Equal(t, "fresh", result.Text)
}

func TestRunTextClonesAndResetsOpenAIProviderOptions(t *testing.T) {
	t.Parallel()

	previousID := "resp-parent"
	storeEnabled := true
	parentOpenAIOpts := &fantasyopenai.ResponsesProviderOptions{
		PreviousResponseID: &previousID,
		Store:              &storeEnabled,
	}
	providerOptions := fantasy.ProviderOptions{
		fantasyopenai.Name: parentOpenAIOpts,
	}

	var observed *fantasyopenai.ResponsesProviderOptions
	_, err := chatnested.RunText(t.Context(), chatnested.RunTextOptions{
		Model: &chattest.FakeModel{
			ProviderName: "openai",
			ModelName:    "gpt-test",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				got, ok := call.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
				require.True(t, ok)
				observed = got
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "answer"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		Messages:        []fantasy.Message{textMessage("question?")},
		ProviderOptions: providerOptions,
	})
	require.NoError(t, err)
	require.NotNil(t, observed)
	require.NotSame(t, parentOpenAIOpts, observed)
	require.Nil(t, observed.PreviousResponseID)
	require.NotNil(t, observed.Store)
	require.False(t, *observed.Store)
	require.NotNil(t, parentOpenAIOpts.PreviousResponseID)
	require.Equal(t, previousID, *parentOpenAIOpts.PreviousResponseID)
	require.True(t, *parentOpenAIOpts.Store)
}

func textMessage(text string) fantasy.Message {
	return fantasy.Message{
		Role: fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func streamFromParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	}
}

func ptrValue(ptr *int64) int64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}

var _ iter.Seq[fantasy.StreamPart] = streamFromParts(nil)
