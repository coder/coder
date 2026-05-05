package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"encoding/base64"
	"errors"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const activeToolName = "read_file"

func validWebSearchProviderMetadataForTest() fantasy.ProviderMetadata {
	return fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{
				{
					URL:              "https://example.com",
					Title:            "Example",
					EncryptedContent: "encrypted",
				},
			},
		},
	}
}

func safeToolCallContent(block fantasy.Content) (fantasy.ToolCallContent, bool) {
	var zero fantasy.ToolCallContent
	switch value := block.(type) {
	case fantasy.ToolCallContent:
		return value, true
	case *fantasy.ToolCallContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func safeToolResultContent(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	var zero fantasy.ToolResultContent
	switch value := block.(type) {
	case fantasy.ToolResultContent:
		return value, true
	case *fantasy.ToolResultContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func safeToolCallPart(part fantasy.MessagePart) (fantasy.ToolCallPart, bool) {
	var zero fantasy.ToolCallPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolCallPart); ok && value == nil {
		return zero, false
	}
	type toolCallPart = fantasy.ToolCallPart
	return fantasy.AsMessagePart[toolCallPart](part)
}

func safeToolResultPart(part fantasy.MessagePart) (fantasy.ToolResultPart, bool) {
	var zero fantasy.ToolResultPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolResultPart); ok && value == nil {
		return zero, false
	}
	type toolResultPart = fantasy.ToolResultPart
	return fantasy.AsMessagePart[toolResultPart](part)
}

func toolCallContentToPart(toolCall fantasy.ToolCallContent) fantasy.ToolCallPart {
	return fantasy.ToolCallPart{
		ToolCallID:       toolCall.ToolCallID,
		ToolName:         toolCall.ToolName,
		Input:            toolCall.Input,
		ProviderExecuted: toolCall.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolCall.ProviderMetadata),
	}
}

func toolResultContentToPart(toolResult fantasy.ToolResultContent) fantasy.ToolResultPart {
	return fantasy.ToolResultPart{
		ToolCallID:       toolResult.ToolCallID,
		Output:           toolResult.Result,
		ProviderExecuted: toolResult.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolResult.ProviderMetadata),
	}
}

func awaitRunResult(ctx context.Context, t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatal("timed out waiting for Run to complete")
		return nil
	}
}

func TestRun_ActiveToolsPrepareBehavior(t *testing.T) {
	t.Parallel()

	var capturedCall fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			capturedCall = call
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	persistStepCalls := 0
	var persistedStep PersistedStep

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys-1"),
			textMessage(fantasy.MessageRoleSystem, "sys-2"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "working"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool(activeToolName),
			newNoopTool("write_file"),
		},
		MaxSteps:             3,
		ActiveTools:          []string{activeToolName},
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistStepCalls++
			persistedStep = step
			return nil
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, persistStepCalls)
	require.True(t, persistedStep.ContextLimit.Valid)
	require.Equal(t, int64(4096), persistedStep.ContextLimit.Int64)
	require.GreaterOrEqual(t, persistedStep.Runtime, time.Duration(0),
		"step runtime should be non-negative")

	require.NotEmpty(t, capturedCall.Prompt)
	require.False(t, containsPromptSentinel(capturedCall.Prompt))
	require.Len(t, capturedCall.Tools, 1)
	require.Equal(t, activeToolName, capturedCall.Tools[0].GetName())

	require.Len(t, capturedCall.Prompt, 5)
	require.False(t, hasAnthropicEphemeralCacheControl(capturedCall.Prompt[0]))
	require.True(t, hasAnthropicEphemeralCacheControl(capturedCall.Prompt[1]))
	require.False(t, hasAnthropicEphemeralCacheControl(capturedCall.Prompt[2]))
	require.True(t, hasAnthropicEphemeralCacheControl(capturedCall.Prompt[3]))
	require.True(t, hasAnthropicEphemeralCacheControl(capturedCall.Prompt[4]))
}

func TestRun_ActiveToolsRejectsDisallowedExecution(t *testing.T) {
	t.Parallel()

	var blockedCalls atomic.Int32
	blockedToolName := "write_file"
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-blocked", ToolCallName: blockedToolName},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-blocked", Delta: `{"path":"/tmp/nope"}`},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-blocked"},
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "tc-blocked",
					ToolCallName:  blockedToolName,
					ToolCallInput: `{"path":"/tmp/nope"}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}), nil
		},
	}

	blockedTool := fantasy.NewAgentTool(
		blockedToolName,
		"blocked tool",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			blockedCalls.Add(1)
			return fantasy.NewTextResponse("should not run"), nil
		},
	)

	var persistedStep PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "try the blocked tool"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool(activeToolName),
			blockedTool,
		},
		ActiveTools: []string{activeToolName},
		MaxSteps:    1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedStep = step
			return nil
		},
	})
	require.NoError(t, err)
	require.Zero(t, blockedCalls.Load(), "disallowed tool must not execute")

	var foundToolError bool
	for _, block := range persistedStep.Content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || toolResult.ToolName != blockedToolName {
			continue
		}
		errResult, ok := toolResult.Result.(fantasy.ToolResultOutputContentError)
		require.True(t, ok)
		assert.EqualError(t, errResult.Error, "Tool not active in this turn: "+blockedToolName)
		foundToolError = true
	}
	require.True(t, foundToolError, "persisted step should include the rejected tool result")
}

func TestRun_ActiveToolsAllowsProviderRunnerExecution(t *testing.T) {
	t.Parallel()

	providerRunnerName := "computer"
	var runnerCalls atomic.Int32
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-provider-runner", ToolCallName: providerRunnerName},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-provider-runner", Delta: `{}`},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-provider-runner"},
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "tc-provider-runner",
					ToolCallName:  providerRunnerName,
					ToolCallInput: `{}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}), nil
		},
	}

	runnerTool := fantasy.NewAgentTool(
		providerRunnerName,
		"provider runner",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			runnerCalls.Add(1)
			return fantasy.NewTextResponse("ran provider runner"), nil
		},
	)

	var persistedStep PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "use the computer"),
		},
		Tools:       []fantasy.AgentTool{newNoopTool(activeToolName)},
		ActiveTools: []string{activeToolName},
		ProviderTools: []ProviderTool{
			{
				Definition: fantasy.FunctionTool{
					Name:        providerRunnerName,
					Description: "provider runner",
					InputSchema: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
				Runner: runnerTool,
			},
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedStep = step
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), runnerCalls.Load(),
		"provider runner should execute even when omitted from active tools")

	var foundToolResult bool
	for _, block := range persistedStep.Content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || toolResult.ToolName != providerRunnerName {
			continue
		}
		textResult, ok := toolResult.Result.(fantasy.ToolResultOutputContentText)
		require.True(t, ok)
		assert.Equal(t, "ran provider runner", textResult.Text)
		foundToolResult = true
	}
	require.True(t, foundToolResult,
		"persisted step should include the provider runner result")
}

func TestRun_ProviderToolResultProviderMetadata(t *testing.T) {
	t.Parallel()

	expectedMetadata := fantasy.ProviderMetadata{
		"openai": &testProviderData{data: map[string]any{
			"detail": "original",
		}},
	}

	tests := []struct {
		name     string
		callback func(fantasy.ToolResponse) fantasy.ProviderMetadata
		want     fantasy.ProviderMetadata
	}{
		{
			name: "callback returns metadata",
			callback: func(fantasy.ToolResponse) fantasy.ProviderMetadata {
				return expectedMetadata
			},
			want: expectedMetadata,
		},
		{
			name: "callback nil",
			want: nil,
		},
		{
			name: "callback returns nil",
			callback: func(fantasy.ToolResponse) fantasy.ProviderMetadata {
				return nil
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			providerRunnerName := "computer"
			model := &chattest.FakeModel{
				ProviderName: "fake",
				StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-provider-runner", ToolCallName: providerRunnerName},
						{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-provider-runner", Delta: `{}`},
						{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-provider-runner"},
						{
							Type:          fantasy.StreamPartTypeToolCall,
							ID:            "tc-provider-runner",
							ToolCallName:  providerRunnerName,
							ToolCallInput: `{}`,
						},
						{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
					}), nil
				},
			}

			runnerTool := fantasy.NewAgentTool(
				providerRunnerName,
				"provider runner",
				func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
					return fantasy.ToolResponse{
						Type:      "image",
						Data:      []byte("image bytes"),
						MediaType: "image/png",
						Content:   "screenshot",
					}, nil
				},
			)

			var persistedStep PersistedStep
			err := Run(context.Background(), RunOptions{
				Model: model,
				Messages: []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "use the computer"),
				},
				ProviderTools: []ProviderTool{
					{
						Definition: fantasy.FunctionTool{
							Name:        providerRunnerName,
							Description: "provider runner",
							InputSchema: map[string]any{
								"type":       "object",
								"properties": map[string]any{},
							},
						},
						Runner:                 runnerTool,
						ResultProviderMetadata: tt.callback,
					},
				},
				MaxSteps: 1,
				PersistStep: func(_ context.Context, step PersistedStep) error {
					persistedStep = step
					return nil
				},
			})
			require.NoError(t, err)

			var foundResult fantasy.ToolResultContent
			for _, block := range persistedStep.Content {
				toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
				if !ok || toolResult.ToolName != providerRunnerName {
					continue
				}
				foundResult = toolResult
				break
			}
			require.NotEmpty(t, foundResult.ToolCallID,
				"persisted step should include the provider runner result")

			mediaResult, ok := foundResult.Result.(fantasy.ToolResultOutputContentMedia)
			require.True(t, ok, "expected media result")
			assert.Equal(t, "image/png", mediaResult.MediaType)
			assert.Equal(t, tt.want, foundResult.ProviderMetadata)

			if tt.want == nil {
				return
			}

			messages := stepResult{content: persistedStep.Content}.toResponseMessages()
			require.Len(t, messages, 2)
			require.Equal(t, fantasy.MessageRoleTool, messages[1].Role)
			require.Len(t, messages[1].Content, 1)

			resultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](messages[1].Content[0])
			require.True(t, ok, "expected outbound tool result part")
			assert.Equal(t, fantasy.ProviderOptions(tt.want), resultPart.ProviderOptions)
		})
	}
}

func TestProcessStepStream_AnthropicUsageMatchesFinalDelta(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "cached response"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{
					Type: fantasy.StreamPartTypeFinish,
					Usage: fantasy.Usage{
						InputTokens:         200,
						OutputTokens:        75,
						TotalTokens:         275,
						CacheCreationTokens: 30,
						CacheReadTokens:     150,
						ReasoningTokens:     0,
					},
					FinishReason: fantasy.FinishReasonStop,
				},
			}), nil
		},
	}

	var persistedStep PersistedStep

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedStep = step
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(200), persistedStep.Usage.InputTokens)
	require.Equal(t, int64(75), persistedStep.Usage.OutputTokens)
	require.Equal(t, int64(275), persistedStep.Usage.TotalTokens)
	require.Equal(t, int64(30), persistedStep.Usage.CacheCreationTokens)
	require.Equal(t, int64(150), persistedStep.Usage.CacheReadTokens)
}

func TestRun_OnRetryEnrichesProvider(t *testing.T) {
	t.Parallel()

	type retryRecord struct {
		attempt    int
		errMsg     string
		classified chatretry.ClassifiedError
		delay      time.Duration
	}

	var records []retryRecord
	calls := 0
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			calls++
			if calls == 1 {
				return nil, xerrors.New("received status 429 from upstream")
			}
			return streamFromParts([]fantasy.StreamPart{{
				Type:         fantasy.StreamPartTypeFinish,
				FinishReason: fantasy.FinishReasonStop,
			}}), nil
		},
	}

	err := Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		OnRetry: func(
			attempt int,
			retryErr error,
			classified chatretry.ClassifiedError,
			delay time.Duration,
		) {
			records = append(records, retryRecord{
				attempt:    attempt,
				errMsg:     retryErr.Error(),
				classified: classified,
				delay:      delay,
			})
		},
	})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, 1, records[0].attempt)
	require.Equal(t, "received status 429 from upstream", records[0].errMsg)
	require.Equal(t, chatretry.Delay(0), records[0].delay)
	require.Equal(t, "openai", records[0].classified.Provider)
	require.Equal(t, chaterror.KindRateLimit, records[0].classified.Kind)
	require.True(t, records[0].classified.Retryable)
	require.Equal(t, 429, records[0].classified.StatusCode)
	require.Equal(
		t,
		"OpenAI is rate limiting requests.",
		records[0].classified.Message,
	)
}

func TestStartupGuard_DisarmAndFireRace(t *testing.T) {
	t.Parallel()

	for range 128 {
		var cancels atomic.Int32
		guard := newStartupGuard(quartz.NewReal(), time.Hour, func(err error) {
			if errors.Is(err, errStartupTimeout) {
				cancels.Add(1)
			}
		})

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			guard.onTimeout()
		}()

		go func() {
			defer wg.Done()
			<-start
			guard.Disarm()
		}()

		close(start)
		wg.Wait()

		guard.onTimeout()
		guard.Disarm()

		require.LessOrEqual(t, cancels.Load(), int32(1))
	}
}

func TestStartupGuard_DisarmPreservesPermanentError(t *testing.T) {
	t.Parallel()

	attemptCtx, cancelAttempt := context.WithCancelCause(context.Background())
	defer cancelAttempt(nil)

	guard := newStartupGuard(quartz.NewReal(), time.Hour, cancelAttempt)
	guard.Disarm()
	guard.onTimeout()

	classified := chaterror.Classify(classifyStartupTimeout(
		attemptCtx,
		"openai",
		xerrors.New("invalid model"),
	))
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	require.False(t, classified.Retryable)
	require.Nil(t, context.Cause(attemptCtx))
}

func TestRun_RetriesStartupTimeoutWhileOpeningStream(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(
		context.Background(),
		testutil.WaitShort,
	)
	defer cancel()

	mClock := quartz.NewMock(t)
	trap := mClock.Trap().AfterFunc("startupGuard")
	defer trap.Close()

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			if attempts == 1 {
				<-ctx.Done()
				attemptCause <- context.Cause(ctx)
				return nil, ctx.Err()
			}
			return streamFromParts([]fantasy.StreamPart{{
				Type:         fantasy.StreamPartTypeFinish,
				FinishReason: fantasy.FinishReasonStop,
			}}), nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunOptions{
			Model:          model,
			MaxSteps:       1,
			StartupTimeout: startupTimeout,
			Clock:          mClock,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			OnRetry: func(
				_ int,
				_ error,
				classified chatretry.ClassifiedError,
				_ time.Duration,
			) {
				retries = append(retries, classified)
			},
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(startupTimeout).MustWait(ctx)
	trap.MustWait(ctx).MustRelease(ctx)

	require.NoError(t, awaitRunResult(ctx, t, done))
	require.Equal(t, 2, attempts)
	require.Len(t, retries, 1)
	require.Equal(t, chaterror.KindStartupTimeout, retries[0].Kind)
	require.True(t, retries[0].Retryable)
	require.Equal(t, "openai", retries[0].Provider)
	require.Equal(
		t,
		"OpenAI did not start responding in time.",
		retries[0].Message,
	)
	select {
	case cause := <-attemptCause:
		require.ErrorIs(t, cause, errStartupTimeout)
	case <-ctx.Done():
		t.Fatal("timed out waiting for startup timeout cause")
	}
}

// TestRun_HTTP2TransportErrorClassifiedAsRetryableTimeout proves the
// provider comes from Model.Provider() (not from sniffing the error
// text) by using an error string with no provider hint and running
// the same assertion across two providers.
func TestRun_HTTP2TransportErrorClassifiedAsRetryableTimeout(t *testing.T) {
	t.Parallel()

	providers := []string{"anthropic", "openai"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()

			const startupTimeout = 5 * time.Millisecond

			ctx, cancel := context.WithTimeout(
				context.Background(),
				testutil.WaitShort,
			)
			defer cancel()

			mClock := quartz.NewMock(t)
			trap := mClock.Trap().AfterFunc("startupGuard")
			defer trap.Close()

			attempts := 0
			var retries []chatretry.ClassifiedError
			model := &chattest.FakeModel{
				ProviderName: provider,
				StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					attempts++
					if attempts == 1 {
						// Bare transport error; Provider must
						// come from Model.Provider().
						return nil, xerrors.New(
							"http2: client connection force closed via ClientConn.Close",
						)
					}
					return streamFromParts([]fantasy.StreamPart{{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonStop,
					}}), nil
				},
			}

			done := make(chan error, 1)
			go func() {
				done <- Run(context.Background(), RunOptions{
					Model:          model,
					MaxSteps:       1,
					StartupTimeout: startupTimeout,
					Clock:          mClock,
					PersistStep: func(_ context.Context, _ PersistedStep) error {
						return nil
					},
					OnRetry: func(
						_ int,
						_ error,
						classified chatretry.ClassifiedError,
						_ time.Duration,
					) {
						retries = append(retries, classified)
					},
				})
			}()

			// One guard per attempt.
			trap.MustWait(ctx).MustRelease(ctx)
			trap.MustWait(ctx).MustRelease(ctx)

			require.NoError(t, awaitRunResult(ctx, t, done))
			require.Equal(t, 2, attempts)
			require.Len(t, retries, 1)
			require.Equal(t, chaterror.KindTimeout, retries[0].Kind, "Kind")
			require.True(t, retries[0].Retryable, "Retryable")
			require.Equal(t, provider, retries[0].Provider, "Provider")
		})
	}
}

func TestRun_RetriesStartupTimeoutBeforeFirstPart(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(
		context.Background(),
		testutil.WaitShort,
	)
	defer cancel()

	mClock := quartz.NewMock(t)
	trap := mClock.Trap().AfterFunc("startupGuard")
	defer trap.Close()

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			if attempts == 1 {
				return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
					<-ctx.Done()
					attemptCause <- context.Cause(ctx)
					_ = yield(fantasy.StreamPart{
						Type:  fantasy.StreamPartTypeError,
						Error: ctx.Err(),
					})
				}), nil
			}
			return streamFromParts([]fantasy.StreamPart{{
				Type:         fantasy.StreamPartTypeFinish,
				FinishReason: fantasy.FinishReasonStop,
			}}), nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunOptions{
			Model:          model,
			MaxSteps:       1,
			StartupTimeout: startupTimeout,
			Clock:          mClock,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			OnRetry: func(
				_ int,
				_ error,
				classified chatretry.ClassifiedError,
				_ time.Duration,
			) {
				retries = append(retries, classified)
			},
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(startupTimeout).MustWait(ctx)
	trap.MustWait(ctx).MustRelease(ctx)

	require.NoError(t, awaitRunResult(ctx, t, done))
	require.Equal(t, 2, attempts)
	require.Len(t, retries, 1)
	require.Equal(t, chaterror.KindStartupTimeout, retries[0].Kind)
	require.True(t, retries[0].Retryable)
	require.Equal(t, "openai", retries[0].Provider)
	require.Equal(
		t,
		"OpenAI did not start responding in time.",
		retries[0].Message,
	)
	select {
	case cause := <-attemptCause:
		require.ErrorIs(t, cause, errStartupTimeout)
	case <-ctx.Done():
		t.Fatal("timed out waiting for startup timeout cause")
	}
}

func TestRun_FirstPartDisarmsStartupTimeout(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(
		context.Background(),
		testutil.WaitShort,
	)
	defer cancel()

	mClock := quartz.NewMock(t)
	trap := mClock.Trap().AfterFunc("startupGuard")

	attempts := 0
	retried := false
	firstPartYielded := make(chan struct{}, 1)
	continueStream := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"}) {
					return
				}
				select {
				case firstPartYielded <- struct{}{}:
				default:
				}

				select {
				case <-continueStream:
				case <-ctx.Done():
					_ = yield(fantasy.StreamPart{
						Type:  fantasy.StreamPartTypeError,
						Error: ctx.Err(),
					})
					return
				}

				parts := []fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}
				for _, part := range parts {
					if !yield(part) {
						return
					}
				}
			}), nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunOptions{
			Model:          model,
			MaxSteps:       1,
			StartupTimeout: startupTimeout,
			Clock:          mClock,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			OnRetry: func(
				_ int,
				_ error,
				_ chatretry.ClassifiedError,
				_ time.Duration,
			) {
				retried = true
			},
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()

	select {
	case <-firstPartYielded:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first stream part")
	}

	mClock.Advance(startupTimeout).MustWait(ctx)
	close(continueStream)

	require.NoError(t, awaitRunResult(ctx, t, done))
	require.Equal(t, 1, attempts)
	require.False(t, retried)
}

func TestRun_PanicInPublishMessagePartReleasesAttempt(t *testing.T) {
	t.Parallel()

	attemptReleased := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			go func() {
				<-ctx.Done()
				close(attemptReleased)
			}()
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "boom"},
			}), nil
		},
	}

	defer func() {
		r := recover()
		require.NotNil(t, r)
		select {
		case <-attemptReleased:
		case <-time.After(time.Second):
			t.Fatal("attempt context was not released after panic")
		}
	}()

	_ = Run(context.Background(), RunOptions{
		Model:                model,
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		PublishMessagePart: func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {
			panic("publish panic")
		},
	})

	t.Fatal("expected Run to panic")
}

func TestRun_RetriesStartupTimeoutWhenStreamClosesSilently(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(
		context.Background(),
		testutil.WaitShort,
	)
	defer cancel()

	mClock := quartz.NewMock(t)
	trap := mClock.Trap().AfterFunc("startupGuard")
	defer trap.Close()

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			if attempts == 1 {
				return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
					<-ctx.Done()
					attemptCause <- context.Cause(ctx)
				}), nil
			}
			return streamFromParts([]fantasy.StreamPart{{
				Type:         fantasy.StreamPartTypeFinish,
				FinishReason: fantasy.FinishReasonStop,
			}}), nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunOptions{
			Model:          model,
			MaxSteps:       1,
			StartupTimeout: startupTimeout,
			Clock:          mClock,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
			OnRetry: func(
				_ int,
				_ error,
				classified chatretry.ClassifiedError,
				_ time.Duration,
			) {
				retries = append(retries, classified)
			},
		})
	}()

	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(startupTimeout).MustWait(ctx)
	trap.MustWait(ctx).MustRelease(ctx)

	require.NoError(t, awaitRunResult(ctx, t, done))
	require.Equal(t, 2, attempts)
	require.Len(t, retries, 1)
	require.Equal(t, chaterror.KindStartupTimeout, retries[0].Kind)
	require.True(t, retries[0].Retryable)
	require.Equal(t, "openai", retries[0].Provider)
	require.Equal(
		t,
		"OpenAI did not start responding in time.",
		retries[0].Message,
	)
	select {
	case cause := <-attemptCause:
		require.ErrorIs(t, cause, errStartupTimeout)
	case <-ctx.Done():
		t.Fatal("timed out waiting for startup timeout cause")
	}
}

func TestRun_InterruptedStepPersistsSyntheticToolResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				parts := []fantasy.StreamPart{
					{
						Type:         fantasy.StreamPartTypeToolInputStart,
						ID:           "interrupt-tool-1",
						ToolCallName: "read_file",
					},
					{
						Type:         fantasy.StreamPartTypeToolInputDelta,
						ID:           "interrupt-tool-1",
						ToolCallName: "read_file",
						Delta:        `{"path":"main.go"`,
					},
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "partial assistant output"},
				}
				for _, part := range parts {
					if !yield(part) {
						return
					}
				}

				select {
				case <-started:
				default:
					close(started)
				}

				<-ctx.Done()
				_ = yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeError,
					Error: ctx.Err(),
				})
			}), nil
		},
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	go func() {
		<-started
		cancel(ErrInterrupted)
	}()

	persistedAssistantCtxErr := xerrors.New("unset")
	var persistedContent []fantasy.Content
	var persistedStep PersistedStep

	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
		},
		MaxSteps: 3,
		PersistStep: func(persistCtx context.Context, step PersistedStep) error {
			persistedAssistantCtxErr = persistCtx.Err()
			persistedContent = append([]fantasy.Content(nil), step.Content...)
			persistedStep = step
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)
	require.NoError(t, persistedAssistantCtxErr)

	require.NotEmpty(t, persistedContent)
	var (
		foundText       bool
		foundToolCall   bool
		foundToolResult bool
	)
	for _, block := range persistedContent {
		if text, ok := fantasy.AsContentType[fantasy.TextContent](block); ok {
			if strings.Contains(text.Text, "partial assistant output") {
				foundText = true
			}
			continue
		}
		if toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok {
			if toolCall.ToolCallID == "interrupt-tool-1" &&
				toolCall.ToolName == "read_file" &&
				strings.Contains(toolCall.Input, `"path":"main.go"`) {
				foundToolCall = true
			}
			continue
		}
		if toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
			if toolResult.ToolCallID == "interrupt-tool-1" &&
				toolResult.ToolName == "read_file" {
				_, isErr := toolResult.Result.(fantasy.ToolResultOutputContentError)
				require.True(t, isErr, "interrupted tool result should be an error")
				foundToolResult = true
			}
		}
	}
	require.True(t, foundText)
	require.True(t, foundToolCall)
	require.True(t, foundToolResult)

	// The interrupted tool was flushed mid-stream (never reached
	// StreamPartTypeToolCall), so it has no call timestamp.
	// But the synthetic error result must have a result timestamp.
	require.Contains(t, persistedStep.ToolResultCreatedAt, "interrupt-tool-1",
		"interrupted tool result must have a result timestamp")
	require.NotContains(t, persistedStep.ToolCallCreatedAt, "interrupt-tool-1",
		"interrupted tool should have no call timestamp (never reached StreamPartTypeToolCall)")
}

func requireToolResultErrorMessage(
	t *testing.T,
	result fantasy.ToolResultContent,
	expected string,
) {
	t.Helper()

	output, ok := result.Result.(fantasy.ToolResultOutputContentError)
	require.Truef(t, ok, "expected error tool result, got %T", result.Result)
	require.Error(t, output.Error)
	require.Equal(t, expected, output.Error.Error())
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

func newNoopTool(name string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		name,
		"test noop tool",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.ToolResponse{}, nil
		},
	)
}

func textMessage(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func requireNoProviderExecutedToolCallContent(t *testing.T, content []fantasy.Content) {
	t.Helper()

	for i, block := range content {
		toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block)
		if ok && toolCall.ProviderExecuted {
			t.Fatalf("content[%d]: unexpected provider-executed call", i)
		}
	}
}

func requireNoProviderExecutedToolResultContent(t *testing.T, content []fantasy.Content) {
	t.Helper()

	for i, block := range content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if ok && toolResult.ProviderExecuted {
			t.Fatalf("content[%d]: unexpected provider-executed result", i)
		}
	}
}

func requireTextPrompt(t *testing.T, prompt []fantasy.Message, text string) fantasy.TextPart {
	t.Helper()

	for _, message := range prompt {
		for _, part := range message.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if ok && textPart.Text == text {
				return textPart
			}
		}
	}
	t.Fatalf("missing prompt text %q", text)
	return fantasy.TextPart{}
}

func requireNoProviderExecutedToolCallPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()

	for i, message := range prompt {
		for j, part := range message.Content {
			toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
			if ok && toolCall.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed call", i, j)
			}
		}
	}
}

func requireTextContent(t *testing.T, content []fantasy.Content, text string) fantasy.TextContent {
	t.Helper()

	for _, block := range content {
		textContent, ok := fantasy.AsContentType[fantasy.TextContent](block)
		if ok && textContent.Text == text {
			return textContent
		}
	}
	t.Fatalf("missing text content %q", text)
	return fantasy.TextContent{}
}

func requireToolCallContent(t *testing.T, content []fantasy.Content, id, name string) fantasy.ToolCallContent {
	t.Helper()

	for _, block := range content {
		toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block)
		if ok && toolCall.ToolCallID == id && toolCall.ToolName == name {
			return toolCall
		}
	}
	t.Fatalf("missing tool call %q", id)
	return fantasy.ToolCallContent{}
}

func requireToolResultContent(t *testing.T, content []fantasy.Content, id, name string) fantasy.ToolResultContent {
	t.Helper()

	for _, block := range content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if ok && toolResult.ToolCallID == id && toolResult.ToolName == name {
			return toolResult
		}
	}
	t.Fatalf("missing tool result %q", id)
	return fantasy.ToolResultContent{}
}

func requireToolResultPrompt(t *testing.T, prompt []fantasy.Message, id string) fantasy.ToolResultPart {
	t.Helper()

	for _, message := range prompt {
		for _, part := range message.Content {
			toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
			if ok && toolResult.ToolCallID == id {
				return toolResult
			}
		}
	}
	t.Fatalf("missing prompt tool result %q", id)
	return fantasy.ToolResultPart{}
}

func requireNoProviderExecutedToolResultPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()

	for i, message := range prompt {
		for j, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed result", i, j)
			}
		}
	}
}

func requireProviderExecutedToolCallPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolCallPart {
	t.Helper()

	for _, message := range prompt {
		for _, part := range message.Content {
			toolCall, ok := safeToolCallPart(part)
			if ok && toolCall.ProviderExecuted && toolCall.ToolCallID == id {
				return toolCall
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool call %q", id)
	return fantasy.ToolCallPart{}
}

func requireProviderExecutedToolResultPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolResultPart {
	t.Helper()

	for _, message := range prompt {
		for _, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted && toolResult.ToolCallID == id {
				return toolResult
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool result %q", id)
	return fantasy.ToolResultPart{}
}

func requireAnthropicProviderToolPromptSafe(t *testing.T, prompt []fantasy.Message) {
	t.Helper()

	require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory(prompt))
}

func requireLogField(t *testing.T, entry slog.SinkEntry, name string) any {
	t.Helper()

	for _, field := range entry.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	t.Fatalf("missing log field %q", name)
	return nil
}

func containsPromptSentinel(prompt []fantasy.Message) bool {
	for _, message := range prompt {
		if message.Role != fantasy.MessageRoleUser || len(message.Content) != 1 {
			continue
		}
		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
		if !ok {
			continue
		}
		if strings.HasPrefix(textPart.Text, "__chatd_agent_prompt_sentinel_") {
			return true
		}
	}
	return false
}

func TestRun_MultiStepToolExecution(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int
	var secondCallPrompt []fantasy.Message

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			switch step {
			case 0:
				// Step 0: produce a tool call.
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
				// Step 1: capture the prompt the loop sent us,
				// then return plain text.
				mu.Lock()
				secondCallPrompt = append([]fantasy.Message(nil), call.Prompt...)
				mu.Unlock()
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "all done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			}
		},
	}

	var persistStepCalls int
	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "please read main.go"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
		},
		MaxSteps: 5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistStepCalls++
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)

	// Stream was called twice: once for the tool-call step,
	// once for the follow-up text step.
	require.Equal(t, 2, streamCalls)

	// PersistStep is called once per step.
	require.Equal(t, 2, persistStepCalls)

	// The second call's prompt must contain the assistant message
	// from step 0 (with the tool call) and a tool-result message.
	require.NotEmpty(t, secondCallPrompt)

	var foundAssistantToolCall bool
	var foundToolResult bool
	for _, msg := range secondCallPrompt {
		if msg.Role == fantasy.MessageRoleAssistant {
			for _, part := range msg.Content {
				if tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
					if tc.ToolCallID == "tc-1" && tc.ToolName == "read_file" {
						foundAssistantToolCall = true
					}
				}
			}
		}
		if msg.Role == fantasy.MessageRoleTool {
			for _, part := range msg.Content {
				if tr, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
					if tr.ToolCallID == "tc-1" {
						foundToolResult = true
					}
				}
			}
		}
	}
	require.True(t, foundAssistantToolCall, "second call prompt should contain assistant tool call from step 0")
	require.True(t, foundToolResult, "second call prompt should contain tool result message")

	// The first persisted step (tool-call step) must carry
	// accurate timestamps for duration computation.
	require.Len(t, persistedSteps, 2)
	toolStep := persistedSteps[0]
	require.Contains(t, toolStep.ToolCallCreatedAt, "tc-1",
		"tool-call step must record when the model emitted the call")
	require.Contains(t, toolStep.ToolResultCreatedAt, "tc-1",
		"tool-call step must record when the tool result was produced")
	require.False(t, toolStep.ToolResultCreatedAt["tc-1"].Before(toolStep.ToolCallCreatedAt["tc-1"]),
		"tool-result timestamp must be >= tool-call timestamp")
}

func TestStopAfterTool_Success(t *testing.T) {
	t.Parallel()

	streamCalls := 0
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-plan", ToolCallName: "propose_plan"},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-plan", Delta: `{"path":"/tmp/plan.md"}`},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-plan"},
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "tc-plan",
					ToolCallName:  "propose_plan",
					ToolCallInput: `{"path":"/tmp/plan.md"}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}), nil
		},
	}

	proposePlanTool := fantasy.NewAgentTool(
		"propose_plan",
		"writes a plan",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("plan saved"), nil
		},
	)

	var persistedSteps []PersistedStep
	persistStepCalls := 0

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "propose a plan"),
		},
		Tools:    []fantasy.AgentTool{proposePlanTool},
		MaxSteps: 5,
		StopAfterTools: map[string]struct{}{
			"propose_plan": {},
		},
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistStepCalls++
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.ErrorIs(t, err, ErrStopAfterTool)
	require.Equal(t, 1, streamCalls)
	require.Equal(t, 1, persistStepCalls)
	require.Len(t, persistedSteps, 1)

	var foundToolResult bool
	for _, block := range persistedSteps[0].Content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || toolResult.ToolName != "propose_plan" {
			continue
		}
		foundToolResult = true
		_, isErr := toolResult.Result.(fantasy.ToolResultOutputContentError)
		require.False(t, isErr, "stop-after-tool should only trigger on successful tool results")
	}
	require.True(t, foundToolResult, "persisted step should include the successful tool result before stopping")
}

func TestStopAfterTool_IgnoresErrorResults(t *testing.T) {
	t.Parallel()

	streamCalls := 0
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			streamCalls++
			if streamCalls == 1 {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-plan", ToolCallName: "propose_plan"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-plan", Delta: `{"path":"/tmp/plan.md"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-plan"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-plan",
						ToolCallName:  "propose_plan",
						ToolCallInput: `{"path":"/tmp/plan.md"}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "tool failed, continue"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	proposePlanTool := fantasy.NewAgentTool(
		"propose_plan",
		"writes a plan",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextErrorResponse("plan failed"), nil
		},
	)

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "propose a plan"),
		},
		Tools:    []fantasy.AgentTool{proposePlanTool},
		MaxSteps: 5,
		StopAfterTools: map[string]struct{}{
			"propose_plan": {},
		},
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
	require.Len(t, persistedSteps, 2)

	var foundToolError bool
	for _, block := range persistedSteps[0].Content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok || toolResult.ToolName != "propose_plan" {
			continue
		}
		_, foundToolError = toolResult.Result.(fantasy.ToolResultOutputContentError)
	}
	require.True(t, foundToolError, "first step should persist the failed tool result")
}

func TestRun_ParallelToolExecutionTimestamps(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			_ = call

			switch step {
			case 0:
				// Step 0: produce two tool calls in one stream.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "read_file"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{"path":"a.go"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-1",
						ToolCallName:  "read_file",
						ToolCallInput: `{"path":"a.go"}`,
					},
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-2", ToolCallName: "write_file"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-2", Delta: `{"path":"b.go"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-2"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-2",
						ToolCallName:  "write_file",
						ToolCallInput: `{"path":"b.go"}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			default:
				// Step 1: return plain text.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "all done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			}
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "do both"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
			newNoopTool("write_file"),
		},
		MaxSteps: 5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)

	// Two steps: tool-call step + text step.
	require.Equal(t, 2, streamCalls)
	require.Len(t, persistedSteps, 2)

	toolStep := persistedSteps[0]

	// Both tool-call IDs must appear in ToolCallCreatedAt.
	require.Contains(t, toolStep.ToolCallCreatedAt, "tc-1",
		"tool-call step must record when tc-1 was emitted")
	require.Contains(t, toolStep.ToolCallCreatedAt, "tc-2",
		"tool-call step must record when tc-2 was emitted")

	// Both tool-call IDs must appear in ToolResultCreatedAt.
	require.Contains(t, toolStep.ToolResultCreatedAt, "tc-1",
		"tool-call step must record when tc-1 result was produced")
	require.Contains(t, toolStep.ToolResultCreatedAt, "tc-2",
		"tool-call step must record when tc-2 result was produced")

	// Result timestamps must be >= call timestamps for both.
	require.False(t, toolStep.ToolResultCreatedAt["tc-1"].Before(toolStep.ToolCallCreatedAt["tc-1"]),
		"tc-1 tool-result timestamp must be >= tool-call timestamp")
	require.False(t, toolStep.ToolResultCreatedAt["tc-2"].Before(toolStep.ToolCallCreatedAt["tc-2"]),
		"tc-2 tool-result timestamp must be >= tool-call timestamp")
}

// TestRun_ExclusiveToolPolicyViolation exercises the full Run() ->
// executeToolsForStep() -> applyExclusiveToolPolicy() wiring. When an
// exclusive tool is called alongside other locally-executable tools,
// neither runner must fire and every call in the batch must receive a
// synthesized policy error that is both persisted and published via
// SSE. This guards against a regression where
// executeToolsForStep's policy call is accidentally removed: the
// pure-unit tests cover the policy function in isolation, but only
// this test catches a broken wiring path.
func TestRun_ExclusiveToolPolicyViolation(t *testing.T) {
	t.Parallel()

	var advisorRuns atomic.Int32
	advisorTool := fantasy.NewAgentTool(
		"advisor",
		"returns strategic guidance",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			advisorRuns.Add(1)
			return fantasy.NewTextResponse(`{"status":"ok"}`), nil
		},
	)
	var readRuns atomic.Int32
	readTool := fantasy.NewAgentTool(
		"read_file",
		"reads a file",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			readRuns.Add(1)
			return fantasy.NewTextResponse(`{"contents":"main"}`), nil
		},
	)

	var mu sync.Mutex
	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			if step == 0 {
				// Step 0: model emits an illegal mixed batch.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "advisor-1", ToolCallName: "advisor"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "advisor-1", Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "advisor-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "advisor-1",
						ToolCallName:  "advisor",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "read-1", ToolCallName: "read_file"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "read-1", Delta: `{"path":"main.go"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "read-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "read-1",
						ToolCallName:  "read_file",
						ToolCallInput: `{"path":"main.go"}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			// Step 1: the loop re-streams after tool results; end the run.
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "ok, retrying"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	var publishedToolParts []codersdk.ChatMessagePart
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "please advise and read"),
		},
		Tools:              []fantasy.AgentTool{advisorTool, readTool},
		ExclusiveToolNames: map[string]bool{"advisor": true},
		MaxSteps:           5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
		PublishMessagePart: func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			if role != codersdk.ChatMessageRoleTool {
				return
			}
			publishedToolParts = append(publishedToolParts, part)
		},
	})
	require.NoError(t, err)

	// Neither runner must have fired: the policy short-circuits
	// before partitioning and execution.
	require.Equal(t, int32(0), advisorRuns.Load(),
		"advisor runner must not fire on mixed batches")
	require.Equal(t, int32(0), readRuns.Load(),
		"read_file runner must not fire on mixed batches")

	// Two steps: the mixed-batch step plus the follow-up stream.
	require.Len(t, persistedSteps, 2)
	firstStep := persistedSteps[0]

	advisorErr, ok := findToolResultByID(firstStep.Content, "advisor-1")
	require.True(t, ok, "persisted step must contain the advisor policy result")
	requireToolResultErrorMessage(t, advisorErr,
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.")

	readErr, ok := findToolResultByID(firstStep.Content, "read-1")
	require.True(t, ok, "persisted step must contain the read_file policy result")
	requireToolResultErrorMessage(t, readErr,
		"this tool was skipped because advisor must run alone in its batch. Retry your tool calls without advisor, or call advisor separately first.")

	// Policy-error results must be SSE-published so the client
	// can render them immediately. Confirm both tool-result parts
	// reached PublishMessagePart with a non-nil CreatedAt, which
	// is the dbtime.Now() stamp the policy branch sets.
	var sawAdvisorPart, sawReadPart bool
	for _, part := range publishedToolParts {
		switch part.ToolCallID {
		case "advisor-1":
			sawAdvisorPart = true
			require.NotNil(t, part.CreatedAt,
				"policy result SSE part must carry the dbtime.Now() timestamp")
		case "read-1":
			sawReadPart = true
			require.NotNil(t, part.CreatedAt,
				"policy result SSE part must carry the dbtime.Now() timestamp")
		}
	}
	require.True(t, sawAdvisorPart, "advisor policy result must be SSE-published")
	require.True(t, sawReadPart, "read_file policy result must be SSE-published")
}

func findToolResultByID(
	content []fantasy.Content,
	toolCallID string,
) (fantasy.ToolResultContent, bool) {
	for _, block := range content {
		tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok {
			continue
		}
		if tr.ToolCallID == toolCallID {
			return tr, true
		}
	}
	return fantasy.ToolResultContent{}, false
}

func TestExclusiveToolPolicy_MixedBatchErrors(t *testing.T) {
	t.Parallel()

	results, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "read-1", ToolName: "read_file", Input: `{"path":"main.go"}`},
		},
		map[string]bool{"advisor": true},
		NopMetrics(),
		"fake",
		"",
	)

	require.True(t, violated)
	require.Len(t, results, 2)
	require.Equal(t, "advisor-1", results[0].ToolCallID)
	require.Equal(t, "read-1", results[1].ToolCallID)
	requireToolResultErrorMessage(
		t,
		results[0],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
	requireToolResultErrorMessage(
		t,
		results[1],
		"this tool was skipped because advisor must run alone in its batch. Retry your tool calls without advisor, or call advisor separately first.",
	)
}

func TestApplyExclusiveToolPolicy_RecordsErrorMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewPedanticRegistry()
	m := NewMetrics(reg)

	_, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "read-1", ToolName: "read_file", Input: `{"path":"main.go"}`},
		},
		map[string]bool{"advisor": true},
		m,
		"fake",
		"claude-test",
	)
	require.True(t, violated)

	require.Equal(t, 1.0, promtestutil.ToFloat64(
		m.ToolErrorsTotal.WithLabelValues("fake", "claude-test", "advisor"),
	))
	require.Equal(t, 1.0, promtestutil.ToFloat64(
		m.ToolErrorsTotal.WithLabelValues("fake", "claude-test", "read_file"),
	))
}

func TestExclusiveToolPolicy_MultipleExclusive(t *testing.T) {
	t.Parallel()

	results, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "advisor-2", ToolName: "advisor", Input: `{"mode":"second-opinion"}`},
		},
		map[string]bool{"advisor": true},
		NopMetrics(),
		"fake",
		"",
	)

	require.True(t, violated)
	require.Len(t, results, 2)
	requireToolResultErrorMessage(
		t,
		results[0],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
	requireToolResultErrorMessage(
		t,
		results[1],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
}

// TestRun_ExclusiveToolPolicyBlocksMixedWithDynamicTool guards the
// exclusive-over-dynamic bypass: the policy must run before the
// built-in vs dynamic partition. If a future refactor moves the
// policy check beneath the partition (so only built-in calls are
// inspected), an exclusive builtin mixed with a dynamic tool would
// still execute locally while the dynamic call is handed off via
// ErrDynamicToolCall, breaking the planning-only contract.
//
// This test has the model emit an exclusive builtin (advisor)
// alongside a dynamic tool (mcp_tool) in the same batch and asserts
// that Run does NOT exit with ErrDynamicToolCall, the advisor
// runner never fires, and both calls receive a synthesized policy
// error.
func TestRun_ExclusiveToolPolicyBlocksMixedWithDynamicTool(t *testing.T) {
	t.Parallel()

	var advisorRuns atomic.Int32
	advisorTool := fantasy.NewAgentTool(
		"advisor",
		"returns strategic guidance",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			advisorRuns.Add(1)
			return fantasy.NewTextResponse(`{"status":"ok"}`), nil
		},
	)

	var mu sync.Mutex
	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			if step == 0 {
				// Step 0: model emits an illegal mixed batch
				// combining an exclusive builtin with a
				// dynamic tool.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "advisor-1", ToolCallName: "advisor"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "advisor-1", Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "advisor-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "advisor-1",
						ToolCallName:  "advisor",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "mcp-1", ToolCallName: "mcp_tool"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "mcp-1", Delta: `{"q":"docs"}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "mcp-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "mcp-1",
						ToolCallName:  "mcp_tool",
						ToolCallInput: `{"q":"docs"}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			// Step 1: after the policy error is fed back,
			// terminate the run so the test assertions have a
			// deterministic exit.
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "retrying"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "please advise and fetch"),
		},
		Tools:              []fantasy.AgentTool{advisorTool},
		DynamicToolNames:   map[string]bool{"mcp_tool": true},
		ExclusiveToolNames: map[string]bool{"advisor": true},
		MaxSteps:           5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	// Run must NOT exit with ErrDynamicToolCall: the policy
	// short-circuits before the dynamic partition so the dynamic
	// call is never handed off for external execution.
	require.NoError(t, err)

	// The advisor runner must not fire on mixed batches; the
	// policy blocks the whole batch including the exclusive tool
	// itself.
	require.Equal(t, int32(0), advisorRuns.Load(),
		"advisor runner must not fire on mixed batches")

	// Two steps: the mixed-batch step with synthesized policy
	// errors plus the follow-up stream that ends the run.
	require.Len(t, persistedSteps, 2)
	firstStep := persistedSteps[0]

	// The persisted step must not record the dynamic tool as
	// pending: the policy-error path returns before
	// persistPendingDynamicStep runs.
	require.Empty(t, firstStep.PendingDynamicToolCalls,
		"policy-rejected batches must not leak dynamic tool calls to the caller")

	advisorErr, ok := findToolResultByID(firstStep.Content, "advisor-1")
	require.True(t, ok, "persisted step must contain the advisor policy result")
	requireToolResultErrorMessage(t, advisorErr,
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.")

	mcpErr, ok := findToolResultByID(firstStep.Content, "mcp-1")
	require.True(t, ok, "persisted step must contain the mcp_tool policy result")
	requireToolResultErrorMessage(t, mcpErr,
		"this tool was skipped because advisor must run alone in its batch. Retry your tool calls without advisor, or call advisor separately first.")
}

// TestRun_ExclusiveToolAloneSucceeds is the happy-path counterpart
// to TestRun_ExclusiveToolPolicyViolation: a single exclusive tool
// emitted alone must actually execute. The `len(toolCalls) <= 1`
// guard in firstExclusiveToolName is the sole mechanism that lets
// solo exclusive-tool calls proceed. If that guard regresses to
// `< 1`, every solo exclusive-tool call would enter an infinite
// policy-error/retry loop, and every unit test on the policy
// function in isolation would still pass. Only this Run()-level
// test catches that regression.
func TestRun_ExclusiveToolAloneSucceeds(t *testing.T) {
	t.Parallel()

	var advisorRuns atomic.Int32
	advisorTool := fantasy.NewAgentTool(
		"advisor",
		"returns strategic guidance",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			advisorRuns.Add(1)
			return fantasy.NewTextResponse(`{"status":"ok"}`), nil
		},
	)

	var mu sync.Mutex
	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			if step == 0 {
				// Step 0: model emits exactly one
				// exclusive-tool call in isolation.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "advisor-1", ToolCallName: "advisor"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "advisor-1", Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "advisor-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "advisor-1",
						ToolCallName:  "advisor",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			// Step 1: the loop re-streams after the tool
			// result; end the run.
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "please advise"),
		},
		Tools:              []fantasy.AgentTool{advisorTool},
		ExclusiveToolNames: map[string]bool{"advisor": true},
		MaxSteps:           5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)

	// The solo exclusive tool must actually execute exactly once.
	require.Equal(t, int32(1), advisorRuns.Load(),
		"solo exclusive-tool call must execute")

	// The first persisted step must contain a non-error tool
	// result for the advisor call, proving the policy did not
	// synthesize an error and the real runner fired.
	require.GreaterOrEqual(t, len(persistedSteps), 1)
	result, ok := findToolResultByID(persistedSteps[0].Content, "advisor-1")
	require.True(t, ok, "persisted step must contain the advisor tool result")
	_, isErr := result.Result.(fantasy.ToolResultOutputContentError)
	require.Falsef(t, isErr,
		"solo exclusive-tool call must produce a real tool result, not a policy error: %+v", result.Result)
}

// TestRun_ExclusiveToolWithProviderExecutedSucceeds guards the
// interaction between the ProviderExecuted filter and the
// exclusive-tool policy. executeToolsForStep builds localCandidates
// by dropping ProviderExecuted calls before passing them to
// applyExclusiveToolPolicy. That filter is the sole mechanism
// preventing a false policy violation when a solo exclusive tool
// appears in a batch where the provider also server-executed a tool
// (for example Anthropic web_search).
//
// If the filter is removed, localCandidates would contain both the
// provider-executed call and the exclusive call. firstExclusiveToolName
// would then see len > 1, find advisor, and return a violation. The
// advisor would never run and the retry loop would burn steps until
// MaxSteps.
//
// This test emits an advisor call alongside a provider-executed
// web_search call (with its provider-emitted result) and asserts the
// advisor runner actually fires.
func TestRun_ExclusiveToolWithProviderExecutedSucceeds(t *testing.T) {
	t.Parallel()

	var advisorRuns atomic.Int32
	advisorTool := fantasy.NewAgentTool(
		"advisor",
		"returns strategic guidance",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			advisorRuns.Add(1)
			return fantasy.NewTextResponse(`{"status":"ok"}`), nil
		},
	)

	var mu sync.Mutex
	var streamCalls int
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			if step == 0 {
				// Step 0: provider server-executed web_search and
				// returned its result inline, plus the model
				// emitted an exclusive advisor call for local
				// execution. The ProviderExecuted filter must
				// drop web_search from the policy check so the
				// advisor is treated as a solo exclusive call.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "ws-1", ToolCallName: "web_search", ProviderExecuted: true},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "ws-1", Delta: `{"query":"coder"}`, ProviderExecuted: true},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "ws-1"},
					{
						Type:             fantasy.StreamPartTypeToolCall,
						ID:               "ws-1",
						ToolCallName:     "web_search",
						ToolCallInput:    `{"query":"coder"}`,
						ProviderExecuted: true,
					},
					{
						Type:             fantasy.StreamPartTypeToolResult,
						ID:               "ws-1",
						ToolCallName:     "web_search",
						ProviderExecuted: true,
					},
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "advisor-1", ToolCallName: "advisor"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "advisor-1", Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "advisor-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "advisor-1",
						ToolCallName:  "advisor",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			// Step 1: end the run after the advisor result is
			// fed back.
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search and then advise"),
		},
		Tools:              []fantasy.AgentTool{advisorTool},
		ExclusiveToolNames: map[string]bool{"advisor": true},
		MaxSteps:           5,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)

	// The advisor must execute exactly once: the ProviderExecuted
	// filter removes web_search from the exclusivity check, so the
	// advisor is treated as a solo exclusive call.
	require.Equal(t, int32(1), advisorRuns.Load(),
		"advisor must execute when the only other call in the batch was provider-executed")

	// The advisor result must be a real tool result, not a
	// synthesized policy error.
	require.GreaterOrEqual(t, len(persistedSteps), 1)
	advisorResult, ok := findToolResultByID(persistedSteps[0].Content, "advisor-1")
	require.True(t, ok, "persisted step must contain the advisor tool result")
	_, isErr := advisorResult.Result.(fantasy.ToolResultOutputContentError)
	require.Falsef(t, isErr,
		"advisor must produce a real tool result, not a policy error: %+v", advisorResult.Result)
}

func TestRun_PersistStepErrorPropagates(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hello"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	persistErr := xerrors.New("database write failed")
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return persistErr
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "database write failed")
}

// TestRun_ShutdownDuringToolExecutionReturnsContextCanceled verifies that
// when the parent context is canceled (simulating server shutdown) while
// a tool is blocked, Run returns context.Canceled, not ErrInterrupted.
// This matters because the caller uses the error type to decide whether
// to set chat status to "pending" (retryable on another worker) vs
// "waiting" (stuck forever).
func TestRun_ShutdownDuringToolExecutionReturnsContextCanceled(t *testing.T) {
	t.Parallel()

	toolStarted := make(chan struct{})

	// Model returns a single tool call, then finishes.
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-block", ToolCallName: "blocking_tool"},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-block", Delta: `{}`},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-block"},
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "tc-block",
					ToolCallName:  "blocking_tool",
					ToolCallInput: `{}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}), nil
		},
	}

	// Tool that blocks until its context is canceled, simulating
	// a long-running operation like wait_agent.
	blockingTool := fantasy.NewAgentTool(
		"blocking_tool",
		"blocks until context canceled",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			close(toolStarted)
			<-ctx.Done()
			return fantasy.ToolResponse{}, ctx.Err()
		},
	)

	// Simulate the server context (parent) and chat context
	// (child). Canceling the parent simulates graceful shutdown.
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	serverCancelDone := make(chan struct{})
	go func() {
		defer close(serverCancelDone)
		<-toolStarted
		t.Logf("tool started, canceling server context to simulate shutdown")
		serverCancel()
	}()

	// persistStep mirrors the FIXED chatd.go code: it only returns
	// ErrInterrupted when the context was actually canceled due to
	// an interruption (cause is ErrInterrupted). For shutdown
	// (plain context.Canceled), it returns the original error so
	// callers can distinguish the two.
	persistStep := func(persistCtx context.Context, _ PersistedStep) error {
		if persistCtx.Err() != nil {
			if errors.Is(context.Cause(persistCtx), ErrInterrupted) {
				return ErrInterrupted
			}
			return persistCtx.Err()
		}
		return nil
	}

	err := Run(serverCtx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "run the blocking tool"),
		},
		Tools:       []fantasy.AgentTool{blockingTool},
		MaxSteps:    3,
		PersistStep: persistStep,
	})
	// Wait for the cancel goroutine to finish to aid flake
	// diagnosis if the test ever hangs.
	<-serverCancelDone

	require.Error(t, err)
	// The error must NOT be ErrInterrupted, it should propagate
	// as context.Canceled so the caller can distinguish shutdown
	// from user interruption. Use assert (not require) so both
	// checks are evaluated even if the first fails.
	assert.NotErrorIs(t, err, ErrInterrupted, "shutdown cancellation must not be converted to ErrInterrupted")
	assert.ErrorIs(t, err, context.Canceled, "shutdown should propagate as context.Canceled")
}

func TestToResponseMessages_ProviderExecutedToolResultInAssistantMessage(t *testing.T) {
	t.Parallel()

	sr := stepResult{
		content: []fantasy.Content{
			// Provider-executed tool call (e.g. web_search).
			fantasy.ToolCallContent{
				ToolCallID:       "provider-tc-1",
				ToolName:         "web_search",
				Input:            `{"query":"coder"}`,
				ProviderExecuted: true,
			},
			// Provider-executed tool result, must stay in
			// assistant message.
			fantasy.ToolResultContent{
				ToolCallID:       "provider-tc-1",
				ToolName:         "web_search",
				ProviderExecuted: true,
				ProviderMetadata: fantasy.ProviderMetadata{"anthropic": nil},
			},
			// Local tool call (e.g. read_file).
			fantasy.ToolCallContent{
				ToolCallID:       "local-tc-1",
				ToolName:         "read_file",
				Input:            `{"path":"main.go"}`,
				ProviderExecuted: false,
			},
			// Local tool result, should go into tool message.
			fantasy.ToolResultContent{
				ToolCallID:       "local-tc-1",
				ToolName:         "read_file",
				Result:           fantasy.ToolResultOutputContentText{Text: "some result"},
				ProviderExecuted: false,
			},
		},
	}

	msgs := sr.toResponseMessages()
	require.Len(t, msgs, 2, "expected assistant + tool messages")

	// First message: assistant role.
	assistantMsg := msgs[0]
	assert.Equal(t, fantasy.MessageRoleAssistant, assistantMsg.Role)
	require.Len(t, assistantMsg.Content, 3,
		"assistant message should have provider ToolCallPart, provider ToolResultPart, and local ToolCallPart")

	// Part 0: provider tool call.
	providerTC, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](assistantMsg.Content[0])
	require.True(t, ok, "part 0 should be ToolCallPart")
	assert.Equal(t, "provider-tc-1", providerTC.ToolCallID)
	assert.True(t, providerTC.ProviderExecuted)

	// Part 1: provider tool result (inline in assistant turn).
	providerTR, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](assistantMsg.Content[1])
	require.True(t, ok, "part 1 should be ToolResultPart")
	assert.Equal(t, "provider-tc-1", providerTR.ToolCallID)
	assert.True(t, providerTR.ProviderExecuted)

	// Part 2: local tool call.
	localTC, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](assistantMsg.Content[2])
	require.True(t, ok, "part 2 should be ToolCallPart")
	assert.Equal(t, "local-tc-1", localTC.ToolCallID)
	assert.False(t, localTC.ProviderExecuted)

	// Second message: tool role.
	toolMsg := msgs[1]
	assert.Equal(t, fantasy.MessageRoleTool, toolMsg.Role)
	require.Len(t, toolMsg.Content, 1,
		"tool message should have only the local ToolResultPart")

	localTR, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](toolMsg.Content[0])
	require.True(t, ok, "tool part should be ToolResultPart")
	assert.Equal(t, "local-tc-1", localTR.ToolCallID)
	assert.False(t, localTR.ProviderExecuted)
}

func TestToResponseMessages_FiltersEmptyTextAndReasoningParts(t *testing.T) {
	t.Parallel()

	sr := stepResult{
		content: []fantasy.Content{
			// Empty text, should be filtered.
			fantasy.TextContent{Text: ""},
			// Whitespace-only text, should be filtered.
			fantasy.TextContent{Text: "   \t\n"},
			// Empty reasoning, should be filtered.
			fantasy.ReasoningContent{Text: ""},
			// Whitespace-only reasoning, should be filtered.
			fantasy.ReasoningContent{Text: "  \n"},
			// Non-empty text, should pass through.
			fantasy.TextContent{Text: "hello world"},
			// Leading/trailing whitespace with content, kept
			// with the original value (not trimmed).
			fantasy.TextContent{Text: "  hello  "},
			// Non-empty reasoning, should pass through.
			fantasy.ReasoningContent{Text: "let me think"},
			// Tool call, should be unaffected by filtering.
			fantasy.ToolCallContent{
				ToolCallID: "tc-1",
				ToolName:   "read_file",
				Input:      `{"path":"main.go"}`,
			},
			// Local tool result, should be unaffected by filtering.
			fantasy.ToolResultContent{
				ToolCallID: "tc-1",
				ToolName:   "read_file",
				Result:     fantasy.ToolResultOutputContentText{Text: "file contents"},
			},
		},
	}

	msgs := sr.toResponseMessages()
	require.Len(t, msgs, 2, "expected assistant + tool messages")

	// First message: assistant role with non-empty text, reasoning,
	// and the tool call. The four empty/whitespace-only parts must
	// have been dropped.
	assistantMsg := msgs[0]
	assert.Equal(t, fantasy.MessageRoleAssistant, assistantMsg.Role)
	require.Len(t, assistantMsg.Content, 4,
		"assistant message should have 2x TextPart, ReasoningPart, and ToolCallPart")

	// Part 0: non-empty text.
	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](assistantMsg.Content[0])
	require.True(t, ok, "part 0 should be TextPart")
	assert.Equal(t, "hello world", textPart.Text)

	// Part 1: padded text, original whitespace preserved.
	paddedPart, ok := fantasy.AsMessagePart[fantasy.TextPart](assistantMsg.Content[1])
	require.True(t, ok, "part 1 should be TextPart")
	assert.Equal(t, "  hello  ", paddedPart.Text)

	// Part 2: non-empty reasoning.
	reasoningPart, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](assistantMsg.Content[2])
	require.True(t, ok, "part 2 should be ReasoningPart")
	assert.Equal(t, "let me think", reasoningPart.Text)

	// Part 3: tool call (unaffected by text/reasoning filtering).
	toolCallPart, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](assistantMsg.Content[3])
	require.True(t, ok, "part 3 should be ToolCallPart")
	assert.Equal(t, "tc-1", toolCallPart.ToolCallID)
	assert.Equal(t, "read_file", toolCallPart.ToolName)

	// Second message: tool role with the local tool result.
	toolMsg := msgs[1]
	assert.Equal(t, fantasy.MessageRoleTool, toolMsg.Role)
	require.Len(t, toolMsg.Content, 1,
		"tool message should have only the local ToolResultPart")

	toolResultPart, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](toolMsg.Content[0])
	require.True(t, ok, "tool part should be ToolResultPart")
	assert.Equal(t, "tc-1", toolResultPart.ToolCallID)
}

func hasAnthropicEphemeralCacheControl(message fantasy.Message) bool {
	if len(message.ProviderOptions) == 0 {
		return false
	}

	options, ok := message.ProviderOptions[fantasyanthropic.Name]
	if !ok {
		return false
	}

	cacheOptions, ok := options.(*fantasyanthropic.ProviderCacheControlOptions)
	return ok && cacheOptions.CacheControl.Type == "ephemeral"
}

// TestRun_InterruptedDuringToolExecutionPersistsStep verifies that when
// tools are executing and the chat is interrupted, the accumulated step
// content (assistant blocks + tool results) is persisted via the
// interrupt-safe path rather than being lost.
func TestRun_InterruptedDuringToolExecutionPersistsStep(t *testing.T) {
	t.Parallel()

	toolStarted := make(chan struct{})

	// Model returns a completed tool call in the stream.
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "calling tool"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeReasoningStart, ID: "reason-1"},
				{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reason-1", Delta: "let me think"},
				{Type: fantasy.StreamPartTypeReasoningEnd, ID: "reason-1"},
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "slow_tool"},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{"key":"value"}`},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
				{
					Type:          fantasy.StreamPartTypeToolCall,
					ID:            "tc-1",
					ToolCallName:  "slow_tool",
					ToolCallInput: `{"key":"value"}`,
				},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
			}), nil
		},
	}

	// Tool that blocks until context is canceled, simulating
	// a long-running operation interrupted by the user.
	slowTool := fantasy.NewAgentTool(
		"slow_tool",
		"blocks until canceled",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			close(toolStarted)
			<-ctx.Done()
			return fantasy.ToolResponse{}, ctx.Err()
		},
	)

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	go func() {
		<-toolStarted
		cancel(ErrInterrupted)
	}()

	var persistedContent []fantasy.Content
	persistedCtxErr := xerrors.New("unset")

	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "run the slow tool"),
		},
		Tools:    []fantasy.AgentTool{slowTool},
		MaxSteps: 3,
		PersistStep: func(persistCtx context.Context, step PersistedStep) error {
			persistedCtxErr = persistCtx.Err()
			persistedContent = append([]fantasy.Content(nil), step.Content...)
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)
	// persistInterruptedStep uses context.WithoutCancel, so the
	// persist callback should see a non-canceled context.
	require.NoError(t, persistedCtxErr)
	require.NotEmpty(t, persistedContent)

	var (
		foundText       bool
		foundReasoning  bool
		foundToolCall   bool
		foundToolResult bool
	)
	for _, block := range persistedContent {
		if text, ok := fantasy.AsContentType[fantasy.TextContent](block); ok {
			if strings.Contains(text.Text, "calling tool") {
				foundText = true
			}
			continue
		}
		if reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](block); ok {
			if strings.Contains(reasoning.Text, "let me think") {
				foundReasoning = true
			}
			continue
		}
		if toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok {
			if toolCall.ToolCallID == "tc-1" && toolCall.ToolName == "slow_tool" {
				foundToolCall = true
			}
			continue
		}
		if toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
			if toolResult.ToolCallID == "tc-1" {
				foundToolResult = true
			}
		}
	}
	require.True(t, foundText, "persisted content should include text from the stream")
	require.True(t, foundReasoning, "persisted content should include reasoning from the stream")
	require.True(t, foundToolCall, "persisted content should include the tool call")
	require.True(t, foundToolResult, "persisted content should include the tool result (error from cancellation)")
}

// TestRun_ProviderExecutedToolResultTimestamps verifies that
// provider-executed tool results (e.g. web search) have their
// timestamps recorded in PersistedStep.ToolResultCreatedAt so
// the persistence layer can stamp CreatedAt on the parts.
func TestRun_ProviderExecutedToolResultTimestamps(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			// Simulate a provider-executed tool call and result
			// (e.g. Anthropic web search) followed by a text
			// response, all in a single stream.
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "ws-1", ToolCallName: "web_search", ProviderExecuted: true},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "ws-1", Delta: `{"query":"coder"}`, ProviderExecuted: true},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "ws-1"},
				{
					Type:             fantasy.StreamPartTypeToolCall,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ToolCallInput:    `{"query":"coder"}`,
					ProviderExecuted: true,
				},
				// Provider-executed tool result, emitted by
				// the provider, not our tool runner.
				{
					Type:             fantasy.StreamPartTypeToolResult,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ProviderExecuted: true,
				},
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "search done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search for coder"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, persistedSteps, 1)

	step := persistedSteps[0]

	// Provider-executed tool call should have a call timestamp.
	require.Contains(t, step.ToolCallCreatedAt, "ws-1",
		"provider-executed tool call must record its timestamp")

	// Provider-executed tool result should have a result
	// timestamp so the frontend can compute duration.
	require.Contains(t, step.ToolResultCreatedAt, "ws-1",
		"provider-executed tool result must record its timestamp")

	require.False(t,
		step.ToolResultCreatedAt["ws-1"].Before(step.ToolCallCreatedAt["ws-1"]),
		"tool-result timestamp must be >= tool-call timestamp")
}

func TestRun_AnthropicDropsUnpairedProviderToolBeforePersist(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		toolName  string
		toolInput string
	}{
		{
			name:      "web_search",
			toolName:  "web_search",
			toolInput: `{"query":"coder"}`,
		},
		{
			name:      "code_execution",
			toolName:  "code_execution",
			toolInput: `{"code":"print(1)"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			model := &chattest.FakeModel{
				ProviderName: fantasyanthropic.Name,
				StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					return streamFromParts([]fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeToolInputStart, ID: "pt-1", ToolCallName: tc.toolName, ProviderExecuted: true},
						{Type: fantasy.StreamPartTypeToolInputDelta, ID: "pt-1", Delta: tc.toolInput, ProviderExecuted: true},
						{Type: fantasy.StreamPartTypeToolInputEnd, ID: "pt-1"},
						{
							Type:             fantasy.StreamPartTypeToolCall,
							ID:               "pt-1",
							ToolCallName:     tc.toolName,
							ToolCallInput:    tc.toolInput,
							ProviderExecuted: true,
						},
						{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
					}), nil
				},
			}

			persistCalls := 0
			err := Run(context.Background(), RunOptions{
				Model: model,
				Messages: []fantasy.Message{
					textMessage(fantasy.MessageRoleUser, "run provider tool"),
				},
				MaxSteps: 1,
				PersistStep: func(_ context.Context, _ PersistedStep) error {
					persistCalls++
					return nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, 0, persistCalls)
		})
	}
}

func TestRun_AnthropicKeepsPairedWebSearchBeforePersist(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeToolInputStart, ID: "ws-1", ToolCallName: "web_search", ProviderExecuted: true},
				{Type: fantasy.StreamPartTypeToolInputDelta, ID: "ws-1", Delta: `{"query":"coder"}`, ProviderExecuted: true},
				{Type: fantasy.StreamPartTypeToolInputEnd, ID: "ws-1"},
				{
					Type:             fantasy.StreamPartTypeToolCall,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ToolCallInput:    `{"query":"coder"}`,
					ProviderExecuted: true,
				},
				{
					Type:             fantasy.StreamPartTypeToolResult,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ProviderExecuted: true,
					ProviderMetadata: validWebSearchProviderMetadataForTest(),
				},
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "search done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search for coder"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, persistedSteps, 1)

	toolCall := requireToolCallContent(t, persistedSteps[0].Content, "ws-1", "web_search")
	require.True(t, toolCall.ProviderExecuted)
	toolResult := requireToolResultContent(t, persistedSteps[0].Content, "ws-1", "web_search")
	require.True(t, toolResult.ProviderExecuted)
	requireTextContent(t, persistedSteps[0].Content, "search done")
}

func TestRun_AnthropicInterruptedWebSearchDoesNotPersistSyntheticResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeToolInputStart,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ProviderExecuted: true,
				}) {
					return
				}
				if !yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeToolInputDelta,
					ID:               "ws-1",
					Delta:            `{"query":"coder"}`,
					ProviderExecuted: true,
				}) {
					return
				}
				close(started)
				<-ctx.Done()
				_ = yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeError,
					Error: ctx.Err(),
				})
			}), nil
		},
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go func() {
		<-started
		cancel(ErrInterrupted)
	}()

	persistCalls := 0
	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search for coder"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			persistCalls++
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)
	require.Equal(t, 0, persistCalls)
}

func TestRun_AnthropicInterruptedProviderToolKeepsLocalSyntheticResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeToolInputStart,
					ID:               "ws-1",
					ToolCallName:     "web_search",
					ProviderExecuted: true,
				}) {
					return
				}
				if !yield(fantasy.StreamPart{
					Type:             fantasy.StreamPartTypeToolInputDelta,
					ID:               "ws-1",
					Delta:            `{"query":"coder"}`,
					ProviderExecuted: true,
				}) {
					return
				}
				if !yield(fantasy.StreamPart{
					Type:         fantasy.StreamPartTypeToolInputStart,
					ID:           "tc-1",
					ToolCallName: "read_file",
				}) {
					return
				}
				if !yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeToolInputDelta,
					ID:    "tc-1",
					Delta: `{"path":"main.go"}`,
				}) {
					return
				}
				close(started)
				<-ctx.Done()
				_ = yield(fantasy.StreamPart{
					Type:  fantasy.StreamPartTypeError,
					Error: ctx.Err(),
				})
			}), nil
		},
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	go func() {
		<-started
		cancel(ErrInterrupted)
	}()

	var persistedSteps []PersistedStep
	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search and read"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)
	require.Len(t, persistedSteps, 1)
	requireNoProviderExecutedToolCallContent(t, persistedSteps[0].Content)
	requireNoProviderExecutedToolResultContent(t, persistedSteps[0].Content)

	toolCall := requireToolCallContent(t, persistedSteps[0].Content, "tc-1", "read_file")
	require.False(t, toolCall.ProviderExecuted)
	toolResult := requireToolResultContent(t, persistedSteps[0].Content, "tc-1", "read_file")
	require.False(t, toolResult.ProviderExecuted)
	_, isErr := toolResult.Result.(fantasy.ToolResultOutputContentError)
	require.True(t, isErr)
}

func TestRun_AnthropicSanitizesProviderToolBeforeRequest(t *testing.T) {
	t.Parallel()

	var capturedPrompt []fantasy.Message
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			capturedPrompt = append([]fantasy.Message(nil), call.Prompt...)
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search for coder"),
			{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{
						ToolCallID:       "ws-1",
						ToolName:         "web_search",
						Input:            `{"query":"coder"}`,
						ProviderExecuted: true,
					},
				},
			},
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, capturedPrompt, 1)
	require.Equal(t, fantasy.MessageRoleUser, capturedPrompt[0].Role)
	require.Len(t, capturedPrompt[0].Content, 2)
	requireNoProviderExecutedToolCallPrompt(t, capturedPrompt)
}

func TestRun_AnthropicSanitizesWebSearchBeforeContinuation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int
	var secondCallPrompt []fantasy.Message
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			switch step {
			case 0:
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "ws-1", ToolCallName: "web_search", ProviderExecuted: true},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "ws-1", Delta: `{"query":"coder"}`, ProviderExecuted: true},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "ws-1"},
					{
						Type:             fantasy.StreamPartTypeToolCall,
						ID:               "ws-1",
						ToolCallName:     "web_search",
						ToolCallInput:    `{"query":"coder"}`,
						ProviderExecuted: true,
					},
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
				mu.Lock()
				secondCallPrompt = append([]fantasy.Message(nil), call.Prompt...)
				mu.Unlock()
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			}
		},
	}

	var persistedSteps []PersistedStep
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "search and read"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
		},
		MaxSteps: 2,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedSteps = append(persistedSteps, step)
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)
	require.Len(t, persistedSteps, 2)
	requireNoProviderExecutedToolCallContent(t, persistedSteps[0].Content)
	requireNoProviderExecutedToolCallPrompt(t, secondCallPrompt)

	toolCall := requireToolCallContent(t, persistedSteps[0].Content, "tc-1", "read_file")
	require.False(t, toolCall.ProviderExecuted)
	toolResult := requireToolResultContent(t, persistedSteps[0].Content, "tc-1", "read_file")
	require.False(t, toolResult.ProviderExecuted)
	promptResult := requireToolResultPrompt(t, secondCallPrompt, "tc-1")
	require.False(t, promptResult.ProviderExecuted)
}

func TestSanitizeAnthropicProviderToolContent(t *testing.T) {
	t.Parallel()

	providerCall := func(id, name, input string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{
			ToolCallID:       id,
			ToolName:         name,
			Input:            input,
			ProviderExecuted: true,
		}
	}
	providerResult := func(id, name string) fantasy.ToolResultContent {
		return fantasy.ToolResultContent{
			ToolCallID:       id,
			ToolName:         name,
			ProviderExecuted: true,
			ProviderMetadata: validWebSearchProviderMetadataForTest(),
			Result:           fantasy.ToolResultOutputContentText{Text: "ok"},
		}
	}
	localCall := func(id, name string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{
			ToolCallID: id,
			ToolName:   name,
			Input:      `{}`,
		}
	}
	localResult := func(id, name string) fantasy.ToolResultContent {
		return fantasy.ToolResultContent{
			ToolCallID: id,
			ToolName:   name,
			Result:     fantasy.ToolResultOutputContentText{Text: "ok"},
		}
	}
	type contentSummary struct {
		providerCalls   []string
		providerResults []string
		localCalls      []string
		localResults    []string
	}
	summarizeContent := func(content []fantasy.Content) contentSummary {
		var summary contentSummary
		for _, block := range content {
			if toolCall, ok := safeToolCallContent(block); ok {
				if toolCall.ProviderExecuted {
					summary.providerCalls = append(summary.providerCalls, toolCall.ToolCallID)
				} else {
					summary.localCalls = append(summary.localCalls, toolCall.ToolCallID)
				}
				continue
			}
			if toolResult, ok := safeToolResultContent(block); ok {
				if toolResult.ProviderExecuted {
					summary.providerResults = append(summary.providerResults, toolResult.ToolCallID)
				} else {
					summary.localResults = append(summary.localResults, toolResult.ToolCallID)
				}
			}
		}
		return summary
	}
	assertProviderHistoryValid := func(t *testing.T, content []fantasy.Content) {
		t.Helper()

		parts := make([]fantasy.MessagePart, 0)
		for _, block := range content {
			if toolCall, ok := safeToolCallContent(block); ok && toolCall.ProviderExecuted {
				parts = append(parts, toolCallContentToPart(toolCall))
				continue
			}
			if toolResult, ok := safeToolResultContent(block); ok && toolResult.ProviderExecuted {
				parts = append(parts, toolResultContentToPart(toolResult))
			}
		}
		if len(parts) == 0 {
			return
		}
		require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory([]fantasy.Message{
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: parts,
			},
		}))
	}

	metadataCall := providerCall("ws-meta", "web_search", `{"query":"coder"}`)
	metadataCall.ProviderMetadata = fantasy.ProviderMetadata{fantasyanthropic.Name: nil}
	metadataResult := providerResult("ws-meta", "web_search")
	metadataResult.ProviderMetadata = fantasy.ProviderMetadata{fantasyanthropic.Name: nil}
	pointerCall := providerCall("ws-pointer", "web_search", `{"query":"coder"}`)
	var nilToolCall *fantasy.ToolCallContent

	testCases := []struct {
		name               string
		provider           string
		content            []fantasy.Content
		wantSummary        contentSummary
		wantRemovedCalls   int
		wantRemovedResults int
		wantTexts          []string
		validateAnthropic  bool
	}{
		{
			name:     "orphan provider result textified",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				fantasy.TextContent{Text: "keep"},
				providerResult("ws-1", "web_search"),
			},
			wantRemovedResults: 1,
			wantTexts:          []string{"keep", "ok"},
			validateAnthropic:  true,
		},
		{
			name:     "result before call removes both provider blocks",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerResult("ws-1", "web_search"),
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "valid web search pair preserved",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
				providerResult("ws-1", "web_search"),
				fantasy.TextContent{Text: "search done"},
			},
			wantSummary: contentSummary{
				providerCalls:   []string{"ws-1"},
				providerResults: []string{"ws-1"},
			},
			wantTexts:         []string{"search done"},
			validateAnthropic: true,
		},
		{
			name:     "invalid JSON provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{`),
				providerResult("ws-1", "web_search"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "empty ID provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("", "web_search", `{"query":"coder"}`),
				providerResult("", "web_search"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "empty tool name provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-empty", "", `{"query":"coder"}`),
				providerResult("ws-empty", ""),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "non web search provider pair drops through serializable helper",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("code-1", "code_execution", `{"code":"print(1)"}`),
				providerResult("code-1", "code_execution"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "mismatched provider result tool name drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-mismatch", "web_search", `{"query":"coder"}`),
				providerResult("ws-mismatch", "code_execution"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "duplicate provider IDs drop all provider content for ID",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("dup-1", "web_search", `{"query":"coder"}`),
				providerResult("dup-1", "web_search"),
				providerCall("dup-1", "web_search", `{"query":"coder"}`),
			},
			wantRemovedCalls:   2,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "mismatched provider flags remove only provider side",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("mix-1", "web_search", `{"query":"coder"}`),
				localResult("mix-1", "web_search"),
				localCall("mix-2", "read_file"),
				providerResult("mix-2", "web_search"),
			},
			wantSummary: contentSummary{
				localCalls:   []string{"mix-2"},
				localResults: []string{"mix-1"},
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "malformed provider metadata textifies result",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				metadataCall,
				metadataResult,
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "pointer and nil pointer variants are handled safely",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				nilToolCall,
				&pointerCall,
				providerResult("ws-pointer", "web_search"),
			},
			wantSummary: contentSummary{
				providerCalls:   []string{"ws-pointer"},
				providerResults: []string{"ws-pointer"},
			},
			validateAnthropic: true,
		},
		{
			name:     "local tool content is unchanged",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				localCall("tc-1", "read_file"),
				localResult("tc-1", "read_file"),
			},
			wantSummary: contentSummary{
				localCalls:   []string{"tc-1"},
				localResults: []string{"tc-1"},
			},
			validateAnthropic: true,
		},
		{
			name:     "non Anthropic provider content is unchanged",
			provider: "fake",
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
			},
			wantSummary: contentSummary{
				providerCalls: []string{"ws-1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sanitized, stats := chatsanitize.SanitizeAnthropicProviderToolContent(tc.provider, tc.content)
			require.Equal(t, tc.wantRemovedCalls, stats.RemovedToolCalls)
			require.Equal(t, tc.wantRemovedResults, stats.RemovedToolResults)
			require.Zero(t, stats.DroppedMessages)

			summary := summarizeContent(sanitized)
			assert.ElementsMatch(t, tc.wantSummary.providerCalls, summary.providerCalls)
			assert.ElementsMatch(t, tc.wantSummary.providerResults, summary.providerResults)
			assert.ElementsMatch(t, tc.wantSummary.localCalls, summary.localCalls)
			assert.ElementsMatch(t, tc.wantSummary.localResults, summary.localResults)
			for _, text := range tc.wantTexts {
				requireTextContent(t, sanitized, text)
			}
			if tc.validateAnthropic {
				assertProviderHistoryValid(t, sanitized)
			}
		})
	}
}

func TestRun_AnthropicProviderToolPreRequestGuard(t *testing.T) {
	t.Parallel()

	webSearchTool := ProviderTool{
		Definition: fantasy.ProviderDefinedTool{
			ID:   "anthropic.web_search",
			Name: "web_search",
		},
	}
	providerPair := func(id string) []fantasy.MessagePart {
		return []fantasy.MessagePart{
			fantasy.ToolCallPart{
				ToolCallID:       id,
				ToolName:         "web_search",
				Input:            `{"query":"coder"}`,
				ProviderExecuted: true,
			},
			fantasy.ToolResultPart{
				ToolCallID:       id,
				Output:           fantasy.ToolResultOutputContentText{Text: "ok"},
				ProviderExecuted: true,
				ProviderOptions:  fantasy.ProviderOptions(validWebSearchProviderMetadataForTest()),
			},
		}
	}
	completionModel := func(capturedPrompt *[]fantasy.Message) *chattest.FakeModel {
		return &chattest.FakeModel{
			ProviderName: fantasyanthropic.Name,
			ModelName:    "claude-test",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				*capturedPrompt = append([]fantasy.Message(nil), call.Prompt...)
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		}
	}

	t.Run("allowed web search survives when provider tool is enabled", func(t *testing.T) {
		t.Parallel()

		var capturedPrompt []fantasy.Message
		err := Run(context.Background(), RunOptions{
			Model: completionModel(&capturedPrompt),
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "search"),
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: providerPair("ws-allowed"),
				},
				textMessage(fantasy.MessageRoleUser, "continue"),
			},
			ProviderTools: []ProviderTool{webSearchTool},
			MaxSteps:      1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
		})
		require.NoError(t, err)

		toolCall := requireProviderExecutedToolCallPrompt(t, capturedPrompt, "ws-allowed")
		require.Equal(t, "web_search", toolCall.ToolName)
		requireProviderExecutedToolResultPrompt(t, capturedPrompt, "ws-allowed")
		requireAnthropicProviderToolPromptSafe(t, capturedPrompt)
	})

	t.Run("web search history survives when provider tool is disabled", func(t *testing.T) {
		t.Parallel()

		var capturedPrompt []fantasy.Message
		err := Run(context.Background(), RunOptions{
			Model: completionModel(&capturedPrompt),
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "search and read"),
				{
					Role: fantasy.MessageRoleAssistant,
					Content: append(providerPair("ws-disabled"), fantasy.ToolCallPart{
						ToolCallID: "tc-1",
						ToolName:   "read_file",
						Input:      `{"path":"main.go"}`,
					}),
				},
				{
					Role: fantasy.MessageRoleTool,
					Content: []fantasy.MessagePart{
						fantasy.ToolResultPart{
							ToolCallID: "tc-1",
							Output:     fantasy.ToolResultOutputContentText{Text: "file"},
						},
					},
				},
				textMessage(fantasy.MessageRoleUser, "continue"),
			},
			MaxSteps: 1,
			PersistStep: func(_ context.Context, _ PersistedStep) error {
				return nil
			},
		})
		require.NoError(t, err)

		requireProviderExecutedToolCallPrompt(t, capturedPrompt, "ws-disabled")
		requireProviderExecutedToolResultPrompt(t, capturedPrompt, "ws-disabled")
		promptResult := requireToolResultPrompt(t, capturedPrompt, "tc-1")
		require.False(t, promptResult.ProviderExecuted)
		requireAnthropicProviderToolPromptSafe(t, capturedPrompt)
	})

	t.Run("direct guard textifies orphaned provider result", func(t *testing.T) {
		t.Parallel()

		guarded := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "keep"},
						fantasy.ToolResultPart{
							ToolCallID:       "ws-orphan",
							Output:           fantasy.ToolResultOutputContentText{Text: "search result"},
							ProviderExecuted: true,
						},
					},
				},
			},
		)

		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, 2)
		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "keep", textPart.Text)
		textPart, ok = fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[1])
		require.True(t, ok)
		require.Equal(t, "search result", textPart.Text)
	})

	t.Run("direct guard leaves valid provider history unchanged", func(t *testing.T) {
		t.Parallel()

		content := []fantasy.MessagePart{fantasy.TextPart{Text: "keep"}}
		content = append(content, providerPair("ws-one")...)
		content = append(content, providerPair("ws-two")...)
		guarded := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{{Role: fantasy.MessageRoleAssistant, Content: content}},
		)

		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, len(content))
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-two")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-two")
	})

	t.Run("direct guard leaves non Anthropic providers unchanged", func(t *testing.T) {
		t.Parallel()

		prompt := []fantasy.Message{
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: providerPair("ws-other-provider"),
			},
		}
		guarded := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			"fake",
			"fake-model",
			prompt,
		)
		require.Equal(t, prompt, guarded)
	})

	t.Run("guard logs removals", func(t *testing.T) {
		t.Parallel()

		logSink := testutil.NewFakeSink(t)
		logger := logSink.Logger()
		logPair := providerPair("ws-log")
		guarded := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			logger,
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						logPair[1],
						logPair[0],
					},
				},
			},
		)

		requireNoProviderExecutedToolCallPrompt(t, guarded)
		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireTextPrompt(t, guarded, "ok")
		entries := logSink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelWarn &&
				e.Message == "removed provider-executed tool history"
		})
		require.Len(t, entries, 1)
		require.Equal(t, "pre_request_guard", requireLogField(t, entries[0], "phase"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_calls"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_results"))
	})
}

// TestRun_PersistStepInterruptedFallback verifies that when the normal
// PersistStep call returns ErrInterrupted (e.g., context canceled in a
// race), the step is retried via the interrupt-safe path.
func TestRun_PersistStepInterruptedFallback(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hello world"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var (
		mu           sync.Mutex
		persistCalls int
		savedContent []fantasy.Content
	)

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			mu.Lock()
			defer mu.Unlock()
			persistCalls++
			if persistCalls == 1 {
				// First call: simulate an interrupt race by
				// returning ErrInterrupted without persisting.
				return ErrInterrupted
			}
			// Second call (from persistInterruptedStep fallback):
			// accept the content.
			savedContent = append([]fantasy.Content(nil), step.Content...)
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, persistCalls, "PersistStep should be called twice: once normally (failing), once via fallback")
	require.NotEmpty(t, savedContent)

	var foundText bool
	for _, block := range savedContent {
		if text, ok := fantasy.AsContentType[fantasy.TextContent](block); ok {
			if strings.Contains(text.Text, "hello world") {
				foundText = true
			}
		}
	}
	require.True(t, foundText, "fallback should persist the text content")
}

func TestRun_PrepareMessagesInjectsSystemContextMidLoop(t *testing.T) {
	t.Parallel()

	const injectedInstruction = "You are working in /home/coder/project. Follow AGENTS.md guidelines."

	var mu sync.Mutex
	var streamCalls int
	var secondCallPrompt []fantasy.Message

	// Step 0 calls a tool. Step 1 sees the injected system message.
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			switch step {
			case 0:
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-1", ToolCallName: "create_workspace"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-1", Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-1"},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-1",
						ToolCallName:  "create_workspace",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			default:
				mu.Lock()
				secondCallPrompt = append([]fantasy.Message(nil), call.Prompt...)
				mu.Unlock()
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			}
		},
	}

	// Simulate: after the tool executes (step 0), instruction
	// becomes available. PrepareMessages injects it before step 1.
	instructionInjected := make(chan struct{})
	var instructionAvailable atomic.Value
	// The tool sets instruction after execution.
	tool := fantasy.NewAgentTool(
		"create_workspace",
		"create a workspace",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			instructionAvailable.Store(injectedInstruction)
			return fantasy.ToolResponse{}, nil
		},
	)

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "create a workspace and open a PR"),
		},
		Tools:    []fantasy.AgentTool{tool},
		MaxSteps: 5,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		PrepareMessages: func(msgs []fantasy.Message) []fantasy.Message {
			select {
			case <-instructionInjected:
				return nil
			default:
			}
			instr, ok := instructionAvailable.Load().(string)
			if !ok || instr == "" {
				return nil
			}
			close(instructionInjected)
			// Insert a system message after existing system messages.
			result := make([]fantasy.Message, 0, len(msgs)+1)
			inserted := false
			for i, msg := range msgs {
				result = append(result, msg)
				if !inserted && msg.Role == fantasy.MessageRoleSystem {
					// Insert after the last system message.
					if i+1 >= len(msgs) || msgs[i+1].Role != fantasy.MessageRoleSystem {
						result = append(result, fantasy.Message{
							Role: fantasy.MessageRoleSystem,
							Content: []fantasy.MessagePart{
								fantasy.TextPart{Text: instr},
							},
						})
						inserted = true
					}
				}
			}
			if !inserted {
				// No system messages, prepend.
				result = append([]fantasy.Message{{
					Role: fantasy.MessageRoleSystem,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: instr},
					},
				}}, result...)
			}
			return result
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, streamCalls)

	// The second LLM call should contain the injected instruction.
	require.NotEmpty(t, secondCallPrompt)
	var foundInstruction bool
	for _, msg := range secondCallPrompt {
		if msg.Role != fantasy.MessageRoleSystem {
			continue
		}
		for _, part := range msg.Content {
			if tp, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
				if strings.Contains(tp.Text, "AGENTS.md") {
					foundInstruction = true
				}
			}
		}
	}
	require.True(t, foundInstruction,
		"step 1 prompt should contain the injected system instruction")
}

func TestRun_PrepareMessagesOnlyFiresOnce(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	// Three steps: tool call, tool call, text. PrepareMessages
	// should inject on step 1 and return nil on step 2.
	model := &chattest.FakeModel{
		ProviderName: "fake",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			step := streamCalls
			streamCalls++
			mu.Unlock()

			if step < 2 {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeToolInputStart, ID: "tc-" + strings.Repeat("x", step+1), ToolCallName: "noop"},
					{Type: fantasy.StreamPartTypeToolInputDelta, ID: "tc-" + strings.Repeat("x", step+1), Delta: `{}`},
					{Type: fantasy.StreamPartTypeToolInputEnd, ID: "tc-" + strings.Repeat("x", step+1)},
					{
						Type:          fantasy.StreamPartTypeToolCall,
						ID:            "tc-" + strings.Repeat("x", step+1),
						ToolCallName:  "noop",
						ToolCallInput: `{}`,
					},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
				}), nil
			}
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var prepareCalls atomic.Int32
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "do something"),
		},
		Tools:    []fantasy.AgentTool{newNoopTool("noop")},
		MaxSteps: 5,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		PrepareMessages: func(msgs []fantasy.Message) []fantasy.Message {
			call := prepareCalls.Add(1)
			if call == 1 {
				// First call: inject a message.
				return append(msgs, fantasy.Message{
					Role:    fantasy.MessageRoleSystem,
					Content: []fantasy.MessagePart{fantasy.TextPart{Text: "injected"}},
				})
			}
			// Subsequent calls: no changes.
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, streamCalls)
	// PrepareMessages is called before each of the 3 steps.
	require.Equal(t, 3, int(prepareCalls.Load()))
}

func TestExecuteSingleTool_MediaBase64Encoding(t *testing.T) {
	t.Parallel()

	originalBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	metrics := NewMetrics(prometheus.NewRegistry())
	logger := slog.Make()

	t.Run("EncodesRawBytesToBase64", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"screenshot",
			"takes a screenshot",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Type:      "image",
					Data:      originalBytes,
					MediaType: "image/jpeg",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"screenshot": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   "screenshot",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"screenshot"},
			map[string]struct{}{},
			nil,
		)

		media, ok := result.Result.(fantasy.ToolResultOutputContentMedia)
		require.True(t, ok, "expected ToolResultOutputContentMedia")
		require.Equal(t, "image/jpeg", media.MediaType)

		decoded, err := base64.StdEncoding.DecodeString(media.Data)
		require.NoError(t, err, "Data should be valid base64")
		require.Equal(t, originalBytes, decoded)
	})

	t.Run("SanitizesInvalidUTF8InContent", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"screenshot",
			"takes a screenshot",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Type:      "image",
					Data:      originalBytes,
					MediaType: "image/png",
					Content:   "hello\xffworld",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"screenshot": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-2",
			ToolName:   "screenshot",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"screenshot"},
			map[string]struct{}{},
			nil,
		)

		media, ok := result.Result.(fantasy.ToolResultOutputContentMedia)
		require.True(t, ok, "expected ToolResultOutputContentMedia")
		require.True(t, utf8.ValidString(media.Text), "Text should be valid UTF-8")
		require.Contains(t, media.Text, "hello")
		require.Contains(t, media.Text, "world")
	})

	t.Run("SanitizesInvalidUTF8InTextResult", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"echo",
			"echoes input",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Content: "hello\xffworld",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"echo": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-3",
			ToolName:   "echo",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"echo"},
			map[string]struct{}{},
			nil,
		)

		textOutput, ok := result.Result.(fantasy.ToolResultOutputContentText)
		require.True(t, ok, "expected ToolResultOutputContentText, got %T", result.Result)
		require.True(t, utf8.ValidString(textOutput.Text), "Text should be valid UTF-8")
		require.Contains(t, textOutput.Text, "hello")
		require.Contains(t, textOutput.Text, "world")
	})
}
