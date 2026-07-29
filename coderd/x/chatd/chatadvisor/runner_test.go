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
	}, nil)
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

func TestAdvisorRunTruncatesLongQuestion(t *testing.T) {
	t.Parallel()

	var capturedQuestion string
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				require.NotEmpty(t, call.Prompt)
				capturedQuestion = singleText(t, call.Prompt[len(call.Prompt)-1])
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "Use the smaller diff."},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	question := strings.Repeat("界", 2001)
	result, err := runtime.RunAdvisor(t.Context(), question, nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, strings.Repeat("界", 2000), capturedQuestion)
}

func TestAdvisorRunStreamsAdviceDeltas(t *testing.T) {
	t.Parallel()

	var deltas []string
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "Use "},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "the smaller "},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "diff."},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   2,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(t.Context(), "what should I do?", nil, &chatadvisor.RunAdvisorOptions{
		OnAdviceDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Use ", "the smaller ", "diff."}, deltas)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "Use the smaller diff.", result.Advice)
	require.Equal(t, 1, result.RemainingUses)
}

func TestAdvisorRunResetsAdviceDeltasOnRetry(t *testing.T) {
	t.Parallel()

	var (
		calls  int
		events []string
	)
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
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
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "fresh advice"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   2,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(t.Context(), "what should I do?", nil, &chatadvisor.RunAdvisorOptions{
		OnAdviceDelta: func(delta string) {
			events = append(events, "delta:"+delta)
		},
		OnAdviceReset: func() {
			events = append(events, "reset")
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"delta:stale ", "reset", "delta:fresh advice"}, events)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "fresh advice", result.Advice)
}

func TestAdvisorRunErrorAfterPartialDelta(t *testing.T) {
	t.Parallel()

	var deltas []string
	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "partial advice"},
					{Type: fantasy.StreamPartTypeError, Error: xerrors.New("boom after partial")},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	result, err := runtime.RunAdvisor(t.Context(), "what should I do?", nil, &chatadvisor.RunAdvisorOptions{
		OnAdviceDelta: func(delta string) {
			deltas = append(deltas, delta)
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"partial advice"}, deltas)
	require.Equal(t, chatadvisor.ResultTypeError, result.Type)
	require.Contains(t, result.Error, "boom after partial")
	require.Equal(t, 1, result.RemainingUses)
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

	first, err := runtime.RunAdvisor(t.Context(), "first?", nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, first.Type)
	require.Equal(t, 0, first.RemainingUses)

	second, err := runtime.RunAdvisor(t.Context(), "second?", nil, nil)
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

	result, err := runtime.RunAdvisor(t.Context(), "what failed?", nil, nil)
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

	failed, err := runtime2.RunAdvisor(t.Context(), "first?", nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeError, failed.Type)
	require.Equal(t, 1, failed.RemainingUses)

	retried, err := runtime2.RunAdvisor(t.Context(), "retry?", nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, retried.Type)
	require.Equal(t, "recovered", retried.Advice)
	require.Equal(t, 0, retried.RemainingUses)
}

func TestAdvisorRunTextlessOutcomeDiagnostics(t *testing.T) {
	t.Parallel()

	// A step without usable text collapses into one error result. The
	// error must describe what the model actually returned so failure
	// modes (tool-call mimicry, reasoning-only turns, truncation) are
	// distinguishable from field reports alone.
	tests := []struct {
		name      string
		parts     []fantasy.StreamPart
		wantError string
	}{
		{
			name: "ReasoningOnly",
			parts: []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeReasoningStart, ID: "r-1"},
				{Type: fantasy.StreamPartTypeReasoningDelta, ID: "r-1", Delta: "I should call the advisor tool."},
				{Type: fantasy.StreamPartTypeReasoningEnd, ID: "r-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			},
			wantError: "advisor produced no text output (finish_reason=stop; parts: reasoning=1)",
		},
		{
			name: "ToolCallOnly",
			parts: []fantasy.StreamPart{
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "call-1",
					ToolCallName:  "advisor",
					ToolCallInput: `{"question":"hi"}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			},
			wantError: "advisor produced no text output (finish_reason=tool-calls; parts: tool_call=1)",
		},
		{
			name: "BlankText",
			parts: []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "   "},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			},
			wantError: "advisor produced no text output (finish_reason=stop; parts: blank_text=1)",
		},
		{
			name: "Empty",
			parts: []fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonLength},
			},
			wantError: "advisor produced no text output (finish_reason=length; parts: none)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
				Model: &chattest.FakeModel{
					ProviderName: "test-provider",
					ModelName:    "test-model",
					StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
						return streamFromParts(testCase.parts), nil
					},
				},
				MaxUsesPerRun:   1,
				MaxOutputTokens: 64,
			})
			require.NoError(t, err)

			result, err := runtime.RunAdvisor(t.Context(), "what should I do?", nil, nil)
			require.NoError(t, err)
			require.Equal(t, chatadvisor.ResultTypeError, result.Type)
			require.Equal(t, testCase.wantError, result.Error)
			// A text-free run must refund its use so the parent can retry.
			require.Equal(t, 1, result.RemainingUses)
		})
	}
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

	parentStore := true
	parentOpts := &fantasyopenai.ResponsesProviderOptions{
		Store: &parentStore,
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

	result, err := runtime.RunAdvisor(t.Context(), "anything?", nil, nil)
	require.NoError(t, err)
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)

	// Parent's OpenAI Responses entry must still carry its Store setting;
	// the advisor must have mutated only its per-call clone, never the
	// shared pointer.
	require.NotNil(t, parentOpts.Store)
	require.True(t, *parentOpts.Store)
}

func TestAdvisorRunDisablesStoreAndIsConsistentAcrossCalls(t *testing.T) {
	t.Parallel()

	parentStore := true
	parentOpts := &fantasyopenai.ResponsesProviderOptions{
		Store: &parentStore,
	}
	parentProviderOpts := fantasy.ProviderOptions{
		fantasyopenai.Name: parentOpts,
	}

	// Snapshot Store at stream time to capture exactly what each call sent.
	// Comparing across calls proves the advisor observes consistent
	// (non-persisted) options each invocation.
	type observedOpts struct {
		store *bool
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
		result, err := runtime.RunAdvisor(t.Context(), fmt.Sprintf("q%d", i), nil, nil)
		require.NoError(t, err)
		require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	}

	require.Len(t, observed, 2)
	for i, snap := range observed {
		// Store must be explicitly disabled so the advisor call leaves no
		// stored response behind on the provider.
		require.NotNil(t, snap.store, "call %d did not disable Store", i)
		require.False(t, *snap.store, "call %d ran with Store enabled", i)
	}

	// The parent's pointer must be untouched across repeated advisor runs.
	require.NotNil(t, parentOpts.Store)
	require.True(t, *parentOpts.Store)
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

func TestBuildAdvisorMessagesTextualizesOrphanToolResult(t *testing.T) {
	t.Parallel()

	// Simulate a truncation cut that lands between the assistant tool-call
	// message and its tool-result. The result keeps its context value as a
	// text note; because no raw tool blocks reach the nested call, there
	// is no provider pairing constraint left to violate. The originating
	// call is unknown, so the note uses the generic form.
	snapshot := []fantasy.Message{
		toolResultMessage("call-1", "ok"),
		textMessage(fantasy.MessageRoleAssistant, "final reply"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// Advisor system + result note + assistant reply + question.
	require.Len(t, messages, 4)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Contains(t, singleText(t, messages[0]), "parent agent")
	require.Equal(t, fantasy.MessageRoleUser, messages[1].Role)
	require.Equal(t, "[A tool run by the parent agent returned: ok]", singleText(t, messages[1]))
	require.Equal(t, fantasy.MessageRoleAssistant, messages[2].Role)
	require.Equal(t, "final reply", singleText(t, messages[2]))
	require.Equal(t, fantasy.MessageRoleUser, messages[3].Role)
	require.Equal(t, "Need advice", singleText(t, messages[3]))

	requireNoRawToolContent(t, messages)
}

func TestBuildAdvisorMessagesTextualizesToolExchanges(t *testing.T) {
	t.Parallel()

	// The nested advisor call defines no tools, so assistant-authored tool
	// artifacts must not reach it: the model imitates them instead of
	// answering. Each call/result pair folds into a single user-role note,
	// assistant text survives, and an assistant message that carried only
	// tool calls disappears entirely.
	snapshot := []fantasy.Message{
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "let me look"},
				fantasy.ToolCallPart{ToolCallID: "call-1", ToolName: "search", Input: `{"q":"x"}`},
			},
		},
		toolResultMessage("call-1", "ok"),
		toolCallAssistantMessage("call-2", "search", `{"q":"y"}`),
		toolResultMessage("call-2", "nope"),
		textMessage(fantasy.MessageRoleAssistant, "done"),
	}

	messages := chatadvisor.BuildAdvisorMessages("Need advice", snapshot)

	// Advisor system + assistant text + note 1 + note 2 + assistant reply
	// + question. The call-only assistant message is gone; its input is
	// preserved inside note 2.
	require.Len(t, messages, 6)
	require.Equal(t, fantasy.MessageRoleSystem, messages[0].Role)
	require.Equal(t, fantasy.MessageRoleAssistant, messages[1].Role)
	require.Equal(t, "let me look", singleText(t, messages[1]))
	require.Equal(t, fantasy.MessageRoleUser, messages[2].Role)
	require.Equal(t,
		`[The parent agent ran the search tool with input {"q":"x"}. Result: ok]`,
		singleText(t, messages[2]))
	require.Equal(t, fantasy.MessageRoleUser, messages[3].Role)
	require.Equal(t,
		`[The parent agent ran the search tool with input {"q":"y"}. Result: nope]`,
		singleText(t, messages[3]))
	require.Equal(t, fantasy.MessageRoleAssistant, messages[4].Role)
	require.Equal(t, "done", singleText(t, messages[4]))
	require.Equal(t, fantasy.MessageRoleUser, messages[5].Role)

	requireNoRawToolContent(t, messages)
}

// requireNoRawToolContent asserts that no tool-role message and no raw tool
// call/result part reaches the nested advisor prompt.
func requireNoRawToolContent(t *testing.T, messages []fantasy.Message) {
	t.Helper()
	for _, msg := range messages {
		require.NotEqual(t, fantasy.MessageRoleTool, msg.Role)
		for _, part := range msg.Content {
			_, isCall := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
			require.False(t, isCall, "raw tool call part leaked into advisor prompt")
			_, isResult := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			require.False(t, isResult, "raw tool result part leaked into advisor prompt")
		}
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
