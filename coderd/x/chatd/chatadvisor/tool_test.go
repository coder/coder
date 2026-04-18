package chatadvisor_test

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestAdvisorToolSuccess(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamResponseFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "Use the smaller diff."},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   2,
		MaxOutputTokens: 128,
	})
	require.NoError(t, err)

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime: runtime,
		GetConversationSnapshot: func() []fantasy.Message {
			return []fantasy.Message{{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "We need a safe fix."},
				},
			}}
		},
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "What's the safest next step?"})
	require.False(t, resp.IsError)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "Use the smaller diff.", result.Advice)
	require.Equal(t, "test-provider/test-model", result.AdvisorModel)
	require.Equal(t, 1, result.RemainingUses)
}

func TestAdvisorToolRejectsEmptyQuestion(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime: mustAdvisorRuntime(t),
		GetConversationSnapshot: func() []fantasy.Message {
			return nil
		},
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: " \t\n "})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "question is required")
}

func TestAdvisorToolRejectsLongQuestion(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime: mustAdvisorRuntime(t),
		GetConversationSnapshot: func() []fantasy.Message {
			return nil
		},
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: strings.Repeat("x", 2001)})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "2000 characters or fewer")
}

func TestAdvisorToolRejectsMissingRuntime(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		GetConversationSnapshot: func() []fantasy.Message {
			return nil
		},
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "Need advice"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "advisor runtime is not configured")
}

func TestAdvisorToolRejectsMissingSnapshotFunc(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{Runtime: mustAdvisorRuntime(t)})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "Need advice"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "conversation snapshot provider is not configured")
}

func mustAdvisorRuntime(t *testing.T) *chatadvisor.Runtime {
	t.Helper()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamResponseFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "fallback advice"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   2,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)
	return runtime
}

func runAdvisorTool(
	t *testing.T,
	tool fantasy.AgentTool,
	args chatadvisor.AdvisorArgs,
) fantasy.ToolResponse {
	t.Helper()

	data, err := json.Marshal(args)
	require.NoError(t, err)

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "advisor",
		Input: string(data),
	})
	require.NoError(t, err)
	return resp
}

func streamResponseFromParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	})
}
