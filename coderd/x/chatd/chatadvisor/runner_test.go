package chatadvisor_test

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestAdvisorRunAdvice(t *testing.T) {
	t.Parallel()

	const (
		question        = "What is the smallest safe change?"
		maxOutputTokens = int64(321)
	)

	var capturedCall fantasy.Call
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				capturedCall = call
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "Take the smallest safe change."},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   2,
		MaxOutputTokens: maxOutputTokens,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(context.Background(), question, []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "existing system"),
		textMessage(fantasy.MessageRoleUser, "hello"),
	})
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "Take the smallest safe change.", result.Advice)
	require.Equal(t, "test-provider/test-model", result.AdvisorModel)
	require.Equal(t, 1, result.RemainingUses)

	require.Empty(t, capturedCall.Tools)
	require.NotNil(t, capturedCall.MaxOutputTokens)
	require.Equal(t, maxOutputTokens, *capturedCall.MaxOutputTokens)
	require.NotEmpty(t, capturedCall.Prompt)
	require.Equal(t, fantasy.MessageRoleUser, capturedCall.Prompt[len(capturedCall.Prompt)-1].Role)
	require.Equal(t, question, singleText(t, capturedCall.Prompt[len(capturedCall.Prompt)-1]))
}

func TestAdvisorRunLimitReached(t *testing.T) {
	t.Parallel()

	var calls int
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				calls++
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "first answer"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	first, err := runtime.RunAdvisor(context.Background(), "first?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, first.Type)

	second, err := runtime.RunAdvisor(context.Background(), "second?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeLimitReached, second.Type)
	require.Equal(t, 0, second.RemainingUses)
	require.Equal(t, 1, calls)
}

func TestAdvisorRunError(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return nil, xerrors.New("boom")
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(context.Background(), "what failed?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeError, result.Type)
	require.Contains(t, result.Error, "boom")
	require.Equal(t, 0, result.RemainingUses)
}

func TestNewRuntimeValidation(t *testing.T) {
	t.Parallel()

	matchingTokens := int64(64)
	mismatchedTokens := int64(32)
	model := &chattest.FakeModel{ProviderName: "test-provider", ModelName: "test-model"}

	tests := []struct {
		name    string
		cfg     chatadvisor.RuntimeConfig
		errText string
	}{
		{
			name:    "NilModel",
			cfg:     chatadvisor.RuntimeConfig{MaxUsesPerRun: 1, MaxOutputTokens: 64},
			errText: "advisor model is required",
		},
		{
			name: "NonPositiveMaxUses",
			cfg: chatadvisor.RuntimeConfig{
				Model:           model,
				MaxUsesPerRun:   0,
				MaxOutputTokens: 64,
			},
			errText: "advisor max uses per run must be positive",
		},
		{
			name: "NonPositiveMaxOutputTokens",
			cfg: chatadvisor.RuntimeConfig{
				Model:           model,
				MaxUsesPerRun:   1,
				MaxOutputTokens: 0,
			},
			errText: "advisor max output tokens must be positive",
		},
		{
			name: "MismatchedModelConfigMaxOutputTokens",
			cfg: chatadvisor.RuntimeConfig{
				Model:           model,
				MaxUsesPerRun:   1,
				MaxOutputTokens: matchingTokens,
				ModelConfig: codersdk.ChatModelCallConfig{
					MaxOutputTokens: &mismatchedTokens,
				},
			},
			errText: "must match runtime max output tokens",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := chatadvisor.NewRuntime(testCase.cfg)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.errText)
		})
	}
}

func TestNewRuntimeDeepClonesOpenAIResponsesProviderOptions(t *testing.T) {
	t.Parallel()

	parentPrevID := "resp_parent_abc123"
	parentOpts := &fantasyopenai.ResponsesProviderOptions{
		PreviousResponseID: &parentPrevID,
	}
	parentProviderOpts := fantasy.ProviderOptions{
		fantasyopenai.Name: parentOpts,
	}

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "advice"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		ProviderOptions: parentProviderOpts,
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(context.Background(), "anything?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)

	// Parent's OpenAI Responses entry must still carry its PreviousResponseID;
	// the advisor's nested chatloop run must not have mutated the shared pointer.
	require.NotNil(t, parentOpts.PreviousResponseID)
	require.Equal(t, parentPrevID, *parentOpts.PreviousResponseID)
}

func TestAdvisorRunStripsChainStateAndIsConsistentAcrossCalls(t *testing.T) {
	t.Parallel()

	parentPrevID := "resp_parent_xyz"
	parentOpts := &fantasyopenai.ResponsesProviderOptions{
		PreviousResponseID: &parentPrevID,
	}
	parentProviderOpts := fantasy.ProviderOptions{
		fantasyopenai.Name: parentOpts,
	}

	// Snapshot PreviousResponseID at stream time, before chatloop has any
	// chance to clear it on the shared map. Comparing across calls proves
	// the advisor observes consistent (non-chained) options each invocation.
	var observedPrevIDs []*string
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				openaiOpts, ok := call.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
				switch {
				case !ok, openaiOpts.PreviousResponseID == nil:
					observedPrevIDs = append(observedPrevIDs, nil)
				default:
					copied := *openaiOpts.PreviousResponseID
					observedPrevIDs = append(observedPrevIDs, &copied)
				}
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "advice"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		ProviderOptions: parentProviderOpts,
		MaxUsesPerRun:   2,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	for i := range 2 {
		result, err := runtime.RunAdvisor(context.Background(), fmt.Sprintf("q%d", i), nil)
		require.NoError(t, err)
		require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	}

	require.Len(t, observedPrevIDs, 2)
	for i, prevID := range observedPrevIDs {
		// Each nested call must run without chain mode so prompts built
		// from full history by BuildAdvisorMessages are accepted.
		require.Nil(t, prevID, "call %d unexpectedly ran in chain mode", i)
	}

	// The parent's pointer must be untouched across repeated advisor runs.
	require.NotNil(t, parentOpts.PreviousResponseID)
	require.Equal(t, parentPrevID, *parentOpts.PreviousResponseID)
}

func TestBuildAdvisorMessagesPreservesSystemAndTruncatesConversation(t *testing.T) {
	t.Parallel()

	snapshot := []fantasy.Message{textMessage(fantasy.MessageRoleSystem, "existing system")}
	for i := range 21 {
		snapshot = append(snapshot, textMessage(fantasy.MessageRoleUser, fmt.Sprintf("msg-%02d", i)))
	}
	snapshot = append(snapshot, textMessage(fantasy.MessageRoleAssistant, strings.Repeat("x", 20000)))

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)
	require.GreaterOrEqual(t, len(messages), 4)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Contains(t, singleText(t, messages[0]), "parent agent")
	require.Equal(t, "existing system", singleText(t, messages[1]))
	require.Equal(t, "msg-01", singleText(t, messages[2]))
	require.Equal(t, "msg-20", singleText(t, messages[len(messages)-2]))
	require.Equal(t, "Need advice", singleText(t, messages[len(messages)-1]))

	for _, msg := range messages {
		require.NotContains(t, singleText(t, msg), strings.Repeat("x", 100))
	}
}

func streamFromParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	})
}

func textMessage(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func singleText(t *testing.T, msg fantasy.Message) string {
	t.Helper()
	require.NotEmpty(t, msg.Content)
	text, ok := fantasy.AsMessagePart[fantasy.TextPart](msg.Content[0])
	require.True(t, ok)
	return text.Text
}
