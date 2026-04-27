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

	result, err := runtime.RunAdvisor(t.Context(), question, []fantasy.Message{
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

	first, err := runtime.RunAdvisor(t.Context(), "first?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, first.Type)
	require.Equal(t, 0, first.RemainingUses)

	second, err := runtime.RunAdvisor(t.Context(), "second?", nil)
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

	result, err := runtime.RunAdvisor(t.Context(), "what failed?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeError, result.Type)
	require.Contains(t, result.Error, "boom")
	// A transient nested run failure must not consume quota: callers
	// can retry up to MaxUsesPerRun times despite the failure.
	require.Equal(t, 1, result.RemainingUses)

	// Confirm the refund left the runtime in a usable state by issuing
	// a successful call after the failure, even though MaxUsesPerRun=1.
	runtime2, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func() func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				var calls int
				return func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					calls++
					if calls == 1 {
						return nil, xerrors.New("boom")
					}
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "recovered"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
						{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
					}), nil
				}
			}(),
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	failed, err := runtime2.RunAdvisor(t.Context(), "first?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeError, failed.Type)
	require.Equal(t, 1, failed.RemainingUses)

	retried, err := runtime2.RunAdvisor(t.Context(), "retry?", nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, retried.Type)
	require.Equal(t, "recovered", retried.Advice)
	require.Equal(t, 0, retried.RemainingUses)
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

	result, err := runtime.RunAdvisor(t.Context(), "anything?", nil)
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

	// Snapshot PreviousResponseID and Store at stream time, before chatloop
	// has any chance to clear them on the shared map. Comparing across calls
	// proves the advisor observes consistent (non-chained, non-persisted)
	// options each invocation.
	type observedOpts struct {
		prevID *string
		store  *bool
	}
	var observed []observedOpts
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				openaiOpts, ok := call.ProviderOptions[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
				if !ok {
					observed = append(observed, observedOpts{})
				} else {
					snap := observedOpts{}
					if openaiOpts.PreviousResponseID != nil {
						copied := *openaiOpts.PreviousResponseID
						snap.prevID = &copied
					}
					if openaiOpts.Store != nil {
						copied := *openaiOpts.Store
						snap.store = &copied
					}
					observed = append(observed, snap)
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
		result, err := runtime.RunAdvisor(t.Context(), fmt.Sprintf("q%d", i), nil)
		require.NoError(t, err)
		require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	}

	require.Len(t, observed, 2)
	for i, snap := range observed {
		// Each nested call must run without chain mode so prompts built
		// from full history by BuildAdvisorMessages are accepted.
		require.Nil(t, snap.prevID, "call %d unexpectedly ran in chain mode", i)
		// Store must be explicitly disabled so the provider does not
		// persist an orphan response that later chain-mode calls would
		// fail to resume.
		require.NotNil(t, snap.store, "call %d did not disable Store", i)
		require.False(t, *snap.store, "call %d ran with Store enabled", i)
	}

	// The parent's pointer must be untouched across repeated advisor runs.
	require.NotNil(t, parentOpts.PreviousResponseID)
	require.Equal(t, parentPrevID, *parentOpts.PreviousResponseID)
}

func TestBuildAdvisorMessagesTruncatesToRecentMessageLimit(t *testing.T) {
	t.Parallel()

	snapshot := []fantasy.Message{textMessage(fantasy.MessageRoleSystem, "existing system")}
	for i := range 25 {
		snapshot = append(snapshot, textMessage(fantasy.MessageRoleUser, fmt.Sprintf("msg-%02d", i)))
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)
	// cloned existing system + advisor system + 20 most recent user messages + question.
	require.Len(t, messages, 23)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, "existing system", singleText(t, messages[0]))
	require.Equal(t, fantasy.MessageRoleSystem, messages[1].Role)
	require.Contains(t, singleText(t, messages[1]), "parent agent")
	require.Equal(t, "msg-05", singleText(t, messages[2]))
	require.Equal(t, "msg-24", singleText(t, messages[len(messages)-2]))
	require.Equal(t, "Need advice", singleText(t, messages[len(messages)-1]))
}

func TestBuildAdvisorMessagesStopsAtOversizedMessage(t *testing.T) {
	t.Parallel()

	// The walk is backward from the end of the snapshot. user-late fits,
	// the oversized assistant message breaks the walk, and user-early is
	// never reached. This preserves contiguity: the advisor never sees a
	// message that references missing context.
	snapshot := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "existing system"),
		textMessage(fantasy.MessageRoleUser, "user-early"),
		textMessage(fantasy.MessageRoleAssistant, strings.Repeat("x", 20000)),
		textMessage(fantasy.MessageRoleUser, "user-late"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)
	require.Len(t, messages, 4)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, "existing system", singleText(t, messages[0]))
	require.Equal(t, fantasy.MessageRoleSystem, messages[1].Role)
	require.Contains(t, singleText(t, messages[1]), "parent agent")
	require.Equal(t, "user-late", singleText(t, messages[2]))
	require.Equal(t, "Need advice", singleText(t, messages[3]))

	for _, msg := range messages {
		require.NotContains(t, singleText(t, msg), strings.Repeat("x", 100))
	}
}

func TestBuildAdvisorMessagesPlacesAdvisorPromptAfterInheritedSystem(t *testing.T) {
	t.Parallel()

	snapshot := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "parent-first"),
		textMessage(fantasy.MessageRoleSystem, "parent-second"),
		textMessage(fantasy.MessageRoleUser, "hello"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// Inherited system messages come first in their original order, then
	// the advisor contract, then the recent tail, then the question.
	// This ordering makes the advisor prompt the last system directive
	// so it wins over conflicting parent instructions.
	require.Len(t, messages, 5)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, "parent-first", singleText(t, messages[0]))
	require.Equal(t, fantasy.MessageRoleSystem, messages[1].Role)
	require.Equal(t, "parent-second", singleText(t, messages[1]))
	require.Equal(t, fantasy.MessageRoleSystem, messages[2].Role)
	require.Contains(t, singleText(t, messages[2]), "parent agent")
	require.Equal(t, fantasy.MessageRoleUser, messages[3].Role)
	require.Equal(t, "hello", singleText(t, messages[3]))
	require.Equal(t, fantasy.MessageRoleUser, messages[4].Role)
	require.Equal(t, "Need advice", singleText(t, messages[4]))
}

func TestBuildAdvisorMessagesDropsOversizedInheritedSystem(t *testing.T) {
	t.Parallel()

	// A single oversized parent system message is skipped so it cannot
	// push the advisor prompt past the model's context window. Smaller
	// system messages that fit the budget survive, as do later non-system
	// messages.
	snapshot := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "small-system"),
		textMessage(fantasy.MessageRoleSystem, strings.Repeat("x", 20000)),
		textMessage(fantasy.MessageRoleUser, "hello"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// small-system + advisor system + recent user + question. The
	// oversized inherited system message must not appear.
	require.Len(t, messages, 4)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, "small-system", singleText(t, messages[0]))
	require.Equal(t, fantasy.MessageRoleSystem, messages[1].Role)
	require.Contains(t, singleText(t, messages[1]), "parent agent")
	require.Equal(t, fantasy.MessageRoleUser, messages[2].Role)
	require.Equal(t, "hello", singleText(t, messages[2]))
	require.Equal(t, fantasy.MessageRoleUser, messages[3].Role)
	require.Equal(t, "Need advice", singleText(t, messages[3]))

	for _, msg := range messages {
		require.NotContains(t, singleText(t, msg), strings.Repeat("x", 100))
	}
}

func TestBuildAdvisorMessagesPrefersNewestSystemDirectivesUnderBudget(t *testing.T) {
	t.Parallel()

	// Two parent system messages together exceed the advisor system byte
	// budget, so one must be dropped. Later directives override earlier
	// ones when they conflict, so the advisor must receive the newest
	// directive and drop the older one. Preserve original order among
	// messages that survive so the parent's intended directive sequence
	// is unchanged.
	const payload = 9000
	snapshot := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "older-"+strings.Repeat("a", payload)),
		textMessage(fantasy.MessageRoleSystem, "newer-"+strings.Repeat("b", payload)),
		textMessage(fantasy.MessageRoleUser, "hello"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// newer parent system + advisor system + recent user + question. The
	// older system message must be dropped because the newer directive
	// consumed the remaining budget.
	require.Len(t, messages, 4)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Contains(t, singleText(t, messages[0]), "newer-")
	require.NotContains(t, singleText(t, messages[0]), "older-")
	require.Equal(t, fantasy.MessageRoleSystem, messages[1].Role)
	require.Contains(t, singleText(t, messages[1]), "parent agent")
	require.Equal(t, fantasy.MessageRoleUser, messages[2].Role)
	require.Equal(t, "hello", singleText(t, messages[2]))
	require.Equal(t, fantasy.MessageRoleUser, messages[3].Role)
	require.Equal(t, "Need advice", singleText(t, messages[3]))
}

func TestBuildAdvisorMessagesDropsOrphanToolResults(t *testing.T) {
	t.Parallel()

	// Simulate a truncation cut that lands between the assistant tool-call
	// message and its tool-result. The resulting recent window should not
	// contain an orphan tool_result referencing a missing tool_use block.
	// Building the window with only [tool_result, assistant_reply] mimics
	// the state produced by the backward walk hitting its byte budget right
	// before the tool-call assistant message.
	snapshot := []fantasy.Message{
		toolResultMessage("call-1", "ok"),
		textMessage(fantasy.MessageRoleAssistant, "final reply"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// Advisor system + assistant reply + question. The orphan tool result
	// must not appear in the advisor prompt.
	require.Len(t, messages, 3)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Contains(t, singleText(t, messages[0]), "parent agent")
	require.Equal(t, fantasy.MessageRoleAssistant, messages[1].Role)
	require.Equal(t, "final reply", singleText(t, messages[1]))
	require.Equal(t, fantasy.MessageRoleUser, messages[2].Role)
	require.Equal(t, "Need advice", singleText(t, messages[2]))

	for _, msg := range messages {
		require.NotEqual(t, fantasy.MessageRoleTool, msg.Role)
	}
}

func TestBuildAdvisorMessagesKeepsPairedToolCallAndResult(t *testing.T) {
	t.Parallel()

	snapshot := []fantasy.Message{
		toolCallAssistantMessage("call-1", "search", `{"q":"x"}`),
		toolResultMessage("call-1", "ok"),
		textMessage(fantasy.MessageRoleAssistant, "done"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// Advisor system + assistant tool call + tool result + assistant reply
	// + question. The matched pair must survive.
	require.Len(t, messages, 5)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, messages[1].Role)
	require.Equal(t, fantasy.MessageRoleTool, messages[2].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, messages[3].Role)
	require.Equal(t, "done", singleText(t, messages[3]))
	require.Equal(t, fantasy.MessageRoleUser, messages[4].Role)
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

func toolCallAssistantMessage(callID, name, input string) fantasy.Message {
	return fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{
				ToolCallID: callID,
				ToolName:   name,
				Input:      input,
			},
		},
	}
}

func toolResultMessage(callID, text string) fantasy.Message {
	return fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{
				ToolCallID: callID,
				Output:     fantasy.ToolResultOutputContentText{Text: text},
			},
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
