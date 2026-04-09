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
	"github.com/coder/coder/v2/codersdk"
)

const activeToolName = "read_file"

func TestRun_ActiveToolsPrepareBehavior(t *testing.T) {
	t.Parallel()

	var capturedCall fantasy.Call
	model := &loopTestModel{
		provider: fantasyanthropic.Name,
		streamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
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
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
		guard := newStartupGuard(time.Hour, func(err error) {
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

	guard := newStartupGuard(time.Hour, cancelAttempt)
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

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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

	err := Run(context.Background(), RunOptions{
		Model:          model,
		MaxSteps:       1,
		StartupTimeout: startupTimeout,
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
	require.NoError(t, err)
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
	require.ErrorIs(t, <-attemptCause, errStartupTimeout)
}

func TestRun_RetriesStartupTimeoutBeforeFirstPart(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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

	err := Run(context.Background(), RunOptions{
		Model:          model,
		MaxSteps:       1,
		StartupTimeout: startupTimeout,
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
	require.NoError(t, err)
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
	require.ErrorIs(t, <-attemptCause, errStartupTimeout)
}

func TestRun_FirstPartDisarmsStartupTimeout(t *testing.T) {
	t.Parallel()

	const startupTimeout = 5 * time.Millisecond

	attempts := 0
	retried := false
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			attempts++
			return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"}) {
					return
				}

				timer := time.NewTimer(startupTimeout * 2)
				defer timer.Stop()

				select {
				case <-ctx.Done():
					_ = yield(fantasy.StreamPart{
						Type:  fantasy.StreamPartTypeError,
						Error: ctx.Err(),
					})
					return
				case <-timer.C:
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

	err := Run(context.Background(), RunOptions{
		Model:          model,
		MaxSteps:       1,
		StartupTimeout: startupTimeout,
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
	require.NoError(t, err)
	require.Equal(t, 1, attempts)
	require.False(t, retried)
}

func TestRun_PanicInPublishMessagePartReleasesAttempt(t *testing.T) {
	t.Parallel()

	attemptReleased := make(chan struct{})
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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

	attempts := 0
	attemptCause := make(chan error, 1)
	var retries []chatretry.ClassifiedError
	model := &loopTestModel{
		provider: "openai",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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

	err := Run(context.Background(), RunOptions{
		Model:          model,
		MaxSteps:       1,
		StartupTimeout: startupTimeout,
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
	require.NoError(t, err)
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
	require.ErrorIs(t, <-attemptCause, errStartupTimeout)
}

func TestRun_InterruptedStepPersistsSyntheticToolResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	model := &loopTestModel{
		provider: "fake",
		streamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
}

type loopTestModel struct {
	provider   string
	model      string
	generateFn func(context.Context, fantasy.Call) (*fantasy.Response, error)
	streamFn   func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)
}

func (m *loopTestModel) Provider() string {
	if m.provider != "" {
		return m.provider
	}
	return "fake"
}

func (m *loopTestModel) Model() string {
	if m.model != "" {
		return m.model
	}
	return "fake"
}

func (m *loopTestModel) Generate(ctx context.Context, call fantasy.Call) (*fantasy.Response, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, call)
	}
	return &fantasy.Response{}, nil
}

func (m *loopTestModel) Stream(ctx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, call)
	}
	return streamFromParts([]fantasy.StreamPart{{
		Type:         fantasy.StreamPartTypeFinish,
		FinishReason: fantasy.FinishReasonStop,
	}}), nil
}

func (*loopTestModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*loopTestModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
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

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
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
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "please read main.go"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool("read_file"),
		},
		MaxSteps: 5,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			persistStepCalls++
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
}

func TestRun_PersistStepErrorPropagates(t *testing.T) {
	t.Parallel()

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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

// TestRun_PersistStepInterruptedFallback verifies that when the normal
// PersistStep call returns ErrInterrupted (e.g., context canceled in a
// race), the step is retried via the interrupt-safe path.
func TestRun_PersistStepInterruptedFallback(t *testing.T) {
	t.Parallel()

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
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
