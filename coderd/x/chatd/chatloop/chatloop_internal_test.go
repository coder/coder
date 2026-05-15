package chatloop

import (
	"context"
	"iter"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestRun_ChainBrokenRecovers(t *testing.T) {
	t.Parallel()

	// Given: a chain-mode run whose previous provider_response_id is present in
	//        our database but no longer recognized by the provider for some reason
	var (
		streamCalls   int
		secondCallOpt fantasy.ProviderOptions
		secondPrompt  []fantasy.Message
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				secondPrompt = call.Prompt
				return finishingStream(), nil
			}
		},
	}

	disableCalls := 0
	reloadCalls := 0
	reloadedHistory := []fantasy.Message{
		{Role: "system", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "sys"}}},
		{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}},
		{Role: "assistant", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hi"}}},
		{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "follow up"}}},
	}

	chainFiltered := []fantasy.Message{
		{Role: "system", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "sys"}}},
		{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "follow up"}}},
	}

	// When: the first attempt fails with the chain-broken error
	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages:             chainFiltered,
		ProviderOptions:      chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {
			disableCalls++
		},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			reloadCalls++
			return reloadedHistory, nil
		},
	})

	// Then: DisableChainMode and ReloadMessages each run once and the
	// retry attempt sends the full reloaded history without
	// previous_response_id.
	require.NoError(t, err)
	require.Equal(t, 2, streamCalls, "exactly two stream attempts (one failure, one success)")
	require.Equal(t, 1, disableCalls, "DisableChainMode called once on chain-broken recovery")
	require.Equal(t, 1, reloadCalls, "ReloadMessages called once on chain-broken recovery")

	require.False(t,
		chatopenai.HasPreviousResponseID(secondCallOpt),
		"second attempt must not carry previous_response_id; it was poisoned",
	)
	require.Equal(t, reloadedHistory, secondPrompt,
		"second attempt must use full reloaded history, not chain-filtered prompt",
	)
}

func TestRun_ChainBrokenRecoveryPreparesReloadedMessages(t *testing.T) {
	t.Parallel()

	var (
		streamCalls   int
		prepareCalls  int
		secondCallOpt fantasy.ProviderOptions
		secondPrompt  []fantasy.Message
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				secondPrompt = call.Prompt
				return finishingStream(), nil
			}
		},
	}

	reloadedHistory := []fantasy.Message{
		textMessage(fantasy.MessageRoleUser, "full history"),
	}

	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "chain-filtered"),
		},
		ProviderOptions: chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			return reloadedHistory, nil
		},
		PrepareMessages: func(msgs []fantasy.Message) []fantasy.Message {
			prepareCalls++
			return append(msgs, textMessage(fantasy.MessageRoleSystem, "prepared"))
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
	require.Equal(t, 2, prepareCalls,
		"reloaded history must be prepared before the retry")
	require.False(t, chatopenai.HasPreviousResponseID(secondCallOpt))
	requireTextPrompt(t, secondPrompt, "full history")
	requireTextPrompt(t, secondPrompt, "prepared")
}

func TestRun_ChainBrokenRecoveryAppliesProviderPromptPrep(t *testing.T) {
	t.Parallel()

	var (
		streamCalls   int
		secondCallOpt fantasy.ProviderOptions
		secondPrompt  []fantasy.Message
	)
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				secondPrompt = call.Prompt
				return finishingStream(), nil
			}
		},
	}

	reloadedHistory := []fantasy.Message{
		textMessage(fantasy.MessageRoleSystem, "sys-1"),
		textMessage(fantasy.MessageRoleSystem, "sys-2"),
		textMessage(fantasy.MessageRoleUser, "hello"),
		textMessage(fantasy.MessageRoleAssistant, "hi"),
		textMessage(fantasy.MessageRoleUser, "follow up"),
	}

	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys-2"),
			textMessage(fantasy.MessageRoleUser, "follow up"),
		},
		ProviderOptions: chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			return reloadedHistory, nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
	require.False(t, chatopenai.HasPreviousResponseID(secondCallOpt))
	require.Len(t, secondPrompt, 5)
	require.False(t, hasAnthropicEphemeralCacheControl(secondPrompt[0]))
	require.True(t, hasAnthropicEphemeralCacheControl(secondPrompt[1]))
	require.False(t, hasAnthropicEphemeralCacheControl(secondPrompt[2]))
	require.True(t, hasAnthropicEphemeralCacheControl(secondPrompt[3]))
	require.True(t, hasAnthropicEphemeralCacheControl(secondPrompt[4]))
}

func TestRun_ChainBrokenReloadWithoutDisableChainModeIsExplicit(t *testing.T) {
	t.Parallel()

	var (
		streamCalls   int
		prepareCalls  int
		reloadCalls   int
		secondCallOpt fantasy.ProviderOptions
		secondPrompt  []fantasy.Message
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				secondPrompt = call.Prompt
				return finishingStream(), nil
			}
		},
	}

	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "chain-filtered"),
		},
		ProviderOptions: chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			reloadCalls++
			return []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "full history"),
			}, nil
		},
		PrepareMessages: func(msgs []fantasy.Message) []fantasy.Message {
			prepareCalls++
			return append(msgs, textMessage(fantasy.MessageRoleSystem, "prepared"))
		},
		// DisableChainMode is intentionally nil. This covers callers
		// whose ReloadMessages does not depend on chain-mode state.
	})

	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
	require.Equal(t, 1, reloadCalls)
	require.Equal(t, 2, prepareCalls)
	require.False(t, chatopenai.HasPreviousResponseID(secondCallOpt))
	requireTextPrompt(t, secondPrompt, "full history")
	requireTextPrompt(t, secondPrompt, "prepared")
}

func TestRun_ChainBrokenComposesWithPostStepChainExit(t *testing.T) {
	t.Parallel()

	// Given a chain-mode run whose recovery succeeds and yields a
	// tool call so the step loop continues
	var (
		mu           sync.Mutex
		streamCalls  int
		capturedOpts []fantasy.ProviderOptions
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			streamCalls++
			attempt := streamCalls
			capturedOpts = append(capturedOpts, call.ProviderOptions)
			mu.Unlock()

			switch attempt {
			case 1:
				// Initial chained attempt: 404 from provider.
				return nil, xerrors.New(chainBrokenErrorMessage)
			case 2:
				// Recovery succeeded; emit a tool call so the
				// step loop continues to a second step.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "read_file"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{"path":"main.go"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-1",
						ToolCallName:  "read_file",
						ToolCallInput: `{"path":"main.go"}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			default:
				// Step 1: end the run.
				return finishingStream(), nil
			}
		},
	}

	// When the second step builds its call from opts.ProviderOptions
	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             3,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hi"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
		},
		ProviderOptions: chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			return []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hi"),
			}, nil
		},
	})

	// Then it must not re-send the poisoned previous_response_id
	// because chain-broken recovery cleared both the current call and
	// subsequent step options.
	require.NoError(t, err)
	require.Equal(t, 3, streamCalls,
		"expected three stream calls: chain-broken failure, recovered tool-call step, follow-up step")
	for i, providerOpts := range capturedOpts[1:] {
		require.False(t,
			chatopenai.HasPreviousResponseID(providerOpts),
			"every stream call after recovery (index %d) must have cleared previous_response_id",
			i+1,
		)
	}
}

func TestRun_ChainBrokenReloadFailureStillClearsChain(t *testing.T) {
	t.Parallel()

	// Given: a chain-mode run whose ReloadMessages callback errors
	var (
		streamCalls   int
		prepareCalls  int
		secondCallOpt fantasy.ProviderOptions
		secondPrompt  []fantasy.Message
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				secondPrompt = call.Prompt
				return finishingStream(), nil
			}
		},
	}

	disableCalls := 0
	chainFiltered := []fantasy.Message{
		{Role: "system", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "sys"}}},
		{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "follow up"}}},
	}

	// When: the chain-broken error fires
	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages:             chainFiltered,
		ProviderOptions:      chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {
			disableCalls++
		},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			return nil, xerrors.New("reload exploded")
		},
		PrepareMessages: func(msgs []fantasy.Message) []fantasy.Message {
			prepareCalls++
			return append(msgs, textMessage(fantasy.MessageRoleSystem, "prepared"))
		},
	})

	// Then: the poisoned previous_response_id is still cleared and
	// DisableChainMode still runs, so the retry has any chance of
	// succeeding against the chain-filtered prompt.
	require.NoError(t, err)
	require.Equal(t, 1, disableCalls)
	require.Equal(t, 1, prepareCalls)
	require.False(t,
		chatopenai.HasPreviousResponseID(secondCallOpt),
		"chain options must still be cleared even when reload fails",
	)
	requireTextPrompt(t, secondPrompt, "follow up")
	requireTextPrompt(t, secondPrompt, "prepared")
}

func TestRun_ChainBrokenRecoveryPrepareFailureReturnsPreparePhaseError(t *testing.T) {
	t.Parallel()

	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		ModelName:    "claude-test",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			return nil, xerrors.New(chainBrokenErrorMessage)
		},
	}

	reloadCalls := 0
	err := Run(context.Background(), RunOptions{
		Model:                model,
		Logger:               slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "chain-filtered"),
		},
		ProviderOptions: chainModeProviderOptions("resp_poisoned"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			reloadCalls++
			return []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "search"),
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						fantasy.ReasoningPart{ProviderOptions: fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{RedactedData: "redacted-payload"}}},
						fantasy.ToolCallPart{ToolCallID: "ws-orphan", ToolName: "web_search", Input: `{"query":"coder"}`, ProviderExecuted: true},
						fantasy.TextPart{Text: "partial"},
					},
				},
				textMessage(fantasy.MessageRoleUser, "continue"),
			}, nil
		},
	})

	require.Error(t, err)
	require.Equal(t, 1, reloadCalls)
	require.Equal(t, 1, streamCalls, "retry must fail before issuing another provider call")
	require.ErrorContains(t, err, "prepare prompt:")
	require.NotContains(t, err.Error(), "stream response:")
	require.Equal(t, chaterror.ClassifiedError{
		Message:   "The chat continuation failed due to an internal state mismatch. This is not a configuration or billing issue. Start a new chat to continue.",
		Detail:    "Anthropic replay diagnostic: match=provider_tool_guard_postcondition_failed.",
		Kind:      codersdk.ChatErrorKindGeneric,
		Provider:  fantasyanthropic.Name,
		Retryable: false,
	}, chaterror.Classify(err))
}

func TestRun_ChainBrokenWithoutChainModeIsSafe(t *testing.T) {
	t.Parallel()

	// Given: a run with no chain-mode options or callbacks
	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				return finishingStream(), nil
			}
		},
	}

	// When: a future provider returns a chain-broken signal,
	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		// No ProviderOptions, no DisableChainMode, no ReloadMessages.
	})

	// Then: the recovery branch must no-op (no panic, no missing
	// callbacks) and the retry runs normally.
	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
}

func TestRun_NonChainBrokenRetryDoesNotTouchChainState(t *testing.T) {
	t.Parallel()

	// Given: a chain-mode run with a still-valid previous_response_id
	var (
		streamCalls   int
		secondCallOpt fantasy.ProviderOptions
	)
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			switch streamCalls {
			case 1:
				return nil, xerrors.New("received status 503 from upstream")
			default:
				secondCallOpt = call.ProviderOptions
				return finishingStream(), nil
			}
		},
	}

	disableCalls := 0
	reloadCalls := 0

	// When: a non-chain-broken retryable error fires (503)
	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		Messages: []fantasy.Message{
			{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hi"}}},
		},
		ProviderOptions: chainModeProviderOptions("resp_still_valid"),
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		DisableChainMode: func() {
			disableCalls++
		},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			reloadCalls++
			return nil, nil
		},
	})

	// Then: chain mode stays engaged, ReloadMessages is not called,
	// and the retry preserves previous_response_id.
	require.NoError(t, err)
	require.Equal(t, 0, disableCalls,
		"non-chain-broken retry must not exit chain mode")
	require.Equal(t, 0, reloadCalls,
		"non-chain-broken retry must not reload history")
	require.True(t,
		chatopenai.HasPreviousResponseID(secondCallOpt),
		"non-chain-broken retry must preserve previous_response_id",
	)
}

func TestProcessStepStreamPreservesReasoningMetadataAcrossNilDelta(t *testing.T) {
	t.Parallel()

	stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningStart, ID: "0"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0", Delta: "thinking"})
		yield(fantasy.StreamPart{
			Type: fantasy.StreamPartTypeReasoningDelta,
			ID:   "0",
			ProviderMetadata: fantasy.ProviderMetadata{
				fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
					Signature: "sig",
				},
			},
		})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0", ProviderMetadata: fantasy.ProviderMetadata{}})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningDelta, ID: "0"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningEnd, ID: "0", ProviderMetadata: fantasy.ProviderMetadata{}})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	})

	result, err := processStepStream(context.Background(), stream, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	require.NoError(t, err)
	require.Len(t, result.content, 1)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Equal(t, "thinking", reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "sig", metadata.Signature)
}

func TestProcessStepStreamPersistsRedactedThinkingOnEnd(t *testing.T) {
	t.Parallel()

	stream := iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		reasoningMetadata := fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
				RedactedData: "redacted-payload",
			},
		}
		yield(fantasy.StreamPart{
			Type:             fantasy.StreamPartTypeReasoningStart,
			ID:               "0",
			ProviderMetadata: reasoningMetadata,
		})
		yield(fantasy.StreamPart{
			Type:             fantasy.StreamPartTypeReasoningEnd,
			ID:               "0",
			ProviderMetadata: reasoningMetadata,
		})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "1"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "1", Delta: "done"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "1"})
		yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
	})

	result, err := processStepStream(context.Background(), stream, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
	require.NoError(t, err)
	require.Len(t, result.content, 2)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}

func TestStepResultToResponseMessagesPreservesEmptySignedReasoning(t *testing.T) {
	t.Parallel()

	result := stepResult{
		content: []fantasy.Content{
			fantasy.ReasoningContent{
				ProviderMetadata: fantasy.ProviderMetadata{
					fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
						RedactedData: "redacted-payload",
					},
				},
			},
			fantasy.TextContent{Text: "done"},
		},
	}

	messages := result.toResponseMessages()

	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 2)
	reasoning, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](messages[0].Content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(reasoning.ProviderOptions)
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}

func TestFlushActiveStatePreservesEmptySignedReasoning(t *testing.T) {
	t.Parallel()

	result := &stepResult{}
	flushActiveState(
		result,
		map[string]string{},
		map[string]reasoningState{
			"signed": {
				options: fantasy.ProviderMetadata{
					fantasyanthropic.Name: &fantasyanthropic.ReasoningOptionMetadata{
						RedactedData: "redacted-payload",
					},
				},
			},
			"empty": {},
		},
		map[string]*fantasy.ToolCallContent{},
		map[string]string{},
	)

	require.Len(t, result.content, 1)
	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](result.content[0])
	require.True(t, ok)
	require.Empty(t, reasoning.Text)
	metadata := fantasyanthropic.GetReasoningMetadata(fantasy.ProviderOptions(reasoning.ProviderMetadata))
	require.NotNil(t, metadata)
	require.Equal(t, "redacted-payload", metadata.RedactedData)
}

// chainBrokenError is what OpenAI returns when previous_response_id
// points at a response it does not have stored.
const chainBrokenErrorMessage = "Previous response with id 'resp_abc' not found."

// finishingStream returns a stream that emits a single Finish part.
// The chatloop treats a finishReason of Stop as "stoppedByModel" and
// exits the per-step loop after persisting.
func finishingStream() fantasy.StreamResponse {
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		yield(fantasy.StreamPart{
			Type:         fantasy.StreamPartTypeFinish,
			FinishReason: fantasy.FinishReasonStop,
		})
	})
}

// chainModeProviderOptions builds a fantasy.ProviderOptions carrying
// the OpenAI Responses options with previous_response_id set, the same
// shape chatd builds when chain mode is active.
func chainModeProviderOptions(previousResponseID string) fantasy.ProviderOptions {
	store := true
	return fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
			Store:              &store,
			PreviousResponseID: &previousResponseID,
		},
	}
}
