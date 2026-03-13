package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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

// TestRun_RetryPersistsOnlySuccessfulAttempt verifies that when a
// retryable error occurs mid-stream, the persisted step content
// comes only from the successful attempt. The failed attempt's
// content must not leak into the database.
func TestRun_RetryPersistsOnlySuccessfulAttempt(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			attempt := streamCalls
			streamCalls++
			mu.Unlock()

			switch attempt {
			case 0:
				// First attempt: stream two text deltas then
				// fail with a retryable 503 error.
				return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
					parts := []fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hello "},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "world"},
						{Type: fantasy.StreamPartTypeError, Error: xerrors.New("status 503: service unavailable")},
					}
					for _, p := range parts {
						if !yield(p) {
							return
						}
					}
				}), nil
			default:
				// Second attempt: succeed with the full response.
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hello "},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "world"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			}
		},
	}

	var (
		onRetryCalls    int
		persistedSteps  []PersistedStep
		persistedStepMu sync.Mutex
	)

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistedStepMu.Lock()
			defer persistedStepMu.Unlock()
			persistedSteps = append(persistedSteps, step)
			return nil
		},
		OnRetry: func(_ int, _ error, _ time.Duration) {
			onRetryCalls++
		},
	})
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 2, streamCalls, "model should have been called twice (one failure + one success)")
	mu.Unlock()

	require.Equal(t, 1, onRetryCalls,
		"OnRetry should be called once between failure and success")

	// The step should be persisted exactly once, containing
	// only the successful attempt's content.
	persistedStepMu.Lock()
	defer persistedStepMu.Unlock()
	require.Len(t, persistedSteps, 1, "exactly one step should be persisted")

	// The persisted content should contain the text from the
	// successful attempt. Count text blocks to verify no
	// duplication from the failed attempt leaked through.
	var textBlockCount int
	for _, block := range persistedSteps[0].Content {
		if _, ok := fantasy.AsContentType[fantasy.TextContent](block); ok {
			textBlockCount++
		}
	}
	require.Equal(t, 1, textBlockCount,
		"persisted step should contain exactly one text block from the successful attempt")
}

func TestRun_RetryCallsOnRetryCallback(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			attempt := streamCalls
			streamCalls++
			mu.Unlock()

			if attempt == 0 {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeError, Error: xerrors.New("status 503: service unavailable")},
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

	type retryRecord struct {
		attempt int
		errMsg  string
		delay   time.Duration
	}
	var retries []retryRecord

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		OnRetry: func(attempt int, retryErr error, delay time.Duration) {
			retries = append(retries, retryRecord{
				attempt: attempt,
				errMsg:  retryErr.Error(),
				delay:   delay,
			})
		},
	})
	require.NoError(t, err)

	require.Len(t, retries, 1)
	assert.Equal(t, 1, retries[0].attempt)
	assert.Contains(t, retries[0].errMsg, "503")
	assert.Equal(t, time.Second, retries[0].delay)
}

// TestRun_RetryCancellationPropagates verifies that canceling
// the context during retry backoff propagates the error cleanly
// through Run. Full exhaustion of all 25 attempts is impractical
// in a unit test due to exponential backoff, so we cancel
// explicitly on the first OnRetry callback.
func TestRun_RetryCancellationPropagates(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			streamCalls++
			mu.Unlock()
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeError, Error: xerrors.New("status 503: overloaded")},
			}), nil
		},
	}

	// Cancel the context from OnRetry so the backoff timer
	// select picks up ctx.Done() immediately, avoiding any
	// real-time dependency in the test.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var retryCalls int

	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		OnRetry: func(_ int, _ error, _ time.Duration) {
			retryCalls++
			cancel()
		},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	mu.Lock()
	require.GreaterOrEqual(t, streamCalls, 1,
		"model should have been called at least once")
	mu.Unlock()
	require.GreaterOrEqual(t, retryCalls, 1,
		"OnRetry should have been called at least once")
}

func TestRun_RetryOnlyForRetryableErrors(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			streamCalls++
			mu.Unlock()
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeError, Error: xerrors.New("401 Unauthorized: invalid api key")},
			}), nil
		},
	}

	var retryCalls int

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		OnRetry: func(_ int, _ error, _ time.Duration) {
			retryCalls++
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "401 Unauthorized")

	mu.Lock()
	require.Equal(t, 1, streamCalls,
		"model should be called exactly once for non-retryable errors")
	mu.Unlock()
	require.Equal(t, 0, retryCalls,
		"OnRetry should not be called for non-retryable errors")
}

func TestRun_RetryStreamErrorBeforeProcessing(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var streamCalls int

	model := &loopTestModel{
		provider: "fake",
		streamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			mu.Lock()
			attempt := streamCalls
			streamCalls++
			mu.Unlock()

			if attempt == 0 {
				// Return error directly from Stream, before
				// any stream processing occurs.
				return nil, xerrors.New("status 502: bad gateway")
			}
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "recovered"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		},
	}

	var retryCalls int

	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps: 1,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			return nil
		},
		OnRetry: func(_ int, _ error, _ time.Duration) {
			retryCalls++
		},
	})
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 2, streamCalls,
		"model should be called twice (error + success)")
	mu.Unlock()
	require.Equal(t, 1, retryCalls,
		"OnRetry should be called once")
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
