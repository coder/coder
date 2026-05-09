package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"iter"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

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

func TestRun_ChainBrokenRecovers(t *testing.T) {
	t.Parallel()

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

func TestRun_ChainBrokenComposesWithPostStepChainExit(t *testing.T) {
	t.Parallel()

	// Chain-broken recovery only fixes the in-flight retry; the
	// existing post-step chain-exit path in Run is what carries the
	// cleared options forward to subsequent steps. This test pins
	// that interaction by running a recovery and then a real second
	// step (forced by emitting a tool call from the recovered
	// attempt) and asserting that no later stream call carries
	// previous_response_id.
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
				return nil, xerrors.New(chainBrokenErrorMessage)
			default:
				secondCallOpt = call.ProviderOptions
				return finishingStream(), nil
			}
		},
	}

	disableCalls := 0
	chainFiltered := []fantasy.Message{
		{Role: "system", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "sys"}}},
		{Role: "user", Content: []fantasy.MessagePart{fantasy.TextPart{Text: "follow up"}}},
	}

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
	})

	// Reload failure must not block the chain-broken recovery: the
	// poisoned previous_response_id is cleared regardless so the
	// retry has any chance of succeeding.
	require.NoError(t, err)
	require.Equal(t, 1, disableCalls)
	require.False(t,
		chatopenai.HasPreviousResponseID(secondCallOpt),
		"chain options must still be cleared even when reload fails",
	)
}

func TestRun_ChainBrokenWithoutChainModeIsSafe(t *testing.T) {
	t.Parallel()

	// Defensive: if a future provider returns a chain-broken signal
	// while chain mode is *not* engaged (no previous_response_id in
	// options), the recovery branch must be a no-op and not panic.
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

	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		// No ProviderOptions, no DisableChainMode, no ReloadMessages.
	})

	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
}

func TestRun_NonChainBrokenRetryDoesNotTouchChainState(t *testing.T) {
	t.Parallel()

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
