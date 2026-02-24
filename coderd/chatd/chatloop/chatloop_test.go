package chatloop

import (
	"context"
	"iter"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
)

const subagentReportToolName = "subagent_report"

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

	_, err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys-1"),
			textMessage(fantasy.MessageRoleSystem, "sys-2"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "working"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		Tools: []fantasy.AgentTool{
			newNoopTool(subagentReportToolName),
			newNoopTool("read_file"),
		},
		MaxSteps:             3,
		ActiveTools:          []string{subagentReportToolName},
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, step PersistedStep) error {
			persistStepCalls++
			persistedStep = step
			return nil
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, persistStepCalls)
	require.Empty(t, persistedStep.ToolResults)
	require.True(t, persistedStep.ContextLimit.Valid)
	require.Equal(t, int64(4096), persistedStep.ContextLimit.Int64)

	require.NotEmpty(t, capturedCall.Prompt)
	require.False(t, containsPromptSentinel(capturedCall.Prompt))
	require.Len(t, capturedCall.Tools, 1)
	require.Equal(t, subagentReportToolName, capturedCall.Tools[0].GetName())

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
	var persistedAssistantContent []fantasy.Content
	persistedToolResults := make([]chatprompt.ToolResultBlock, 0, 1)

	_, err := Run(ctx, RunOptions{
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
			persistedAssistantContent = append([]fantasy.Content(nil), step.AssistantContent...)
			persistedToolResults = append(
				persistedToolResults,
				step.ToolResults...,
			)
			return nil
		},
	})
	require.ErrorIs(t, err, ErrInterrupted)
	require.NoError(t, persistedAssistantCtxErr)

	require.NotEmpty(t, persistedAssistantContent)
	var (
		foundText     bool
		foundToolCall bool
	)
	for _, block := range persistedAssistantContent {
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
		}
	}
	require.True(t, foundText)
	require.True(t, foundToolCall)

	require.Len(t, persistedToolResults, 1)
	require.Equal(t, "interrupt-tool-1", persistedToolResults[0].ToolCallID)
	require.Equal(t, "read_file", persistedToolResults[0].ToolName)
	require.True(t, persistedToolResults[0].IsError)

	resultMap, ok := persistedToolResults[0].Result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "interrupted", resultMap["status"])

	errorText, ok := resultMap["error"].(string)
	require.True(t, ok)
	require.Contains(t, strings.ToLower(errorText), "interrupted")
}

type loopTestModel struct {
	provider string
	model    string
	streamFn func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)
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

func (*loopTestModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
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
