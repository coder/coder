package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"iter"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
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
