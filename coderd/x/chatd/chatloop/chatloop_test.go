package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const activeToolName = "read_file"

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
		"OpenAI is rate limiting requests (HTTP 429).",
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
// a tool is blocked, Run returns context.Canceled — not ErrInterrupted.
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
	// The error must NOT be ErrInterrupted — it should propagate
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
			// Provider-executed tool result — must stay in
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
			// Local tool result — should go into tool message.
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
			// Empty text — should be filtered.
			fantasy.TextContent{Text: ""},
			// Whitespace-only text — should be filtered.
			fantasy.TextContent{Text: "   \t\n"},
			// Empty reasoning — should be filtered.
			fantasy.ReasoningContent{Text: ""},
			// Whitespace-only reasoning — should be filtered.
			fantasy.ReasoningContent{Text: "  \n"},
			// Non-empty text — should pass through.
			fantasy.TextContent{Text: "hello world"},
			// Leading/trailing whitespace with content — kept
			// with the original value (not trimmed).
			fantasy.TextContent{Text: "  hello  "},
			// Non-empty reasoning — should pass through.
			fantasy.ReasoningContent{Text: "let me think"},
			// Tool call — should be unaffected by filtering.
			fantasy.ToolCallContent{
				ToolCallID: "tc-1",
				ToolName:   "read_file",
				Input:      `{"path":"main.go"}`,
			},
			// Local tool result — should be unaffected by filtering.
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

	// Part 1: padded text — original whitespace preserved.
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
			// response — all in a single stream.
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
				// Provider-executed tool result — emitted by
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
				// No system messages — prepend.
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
