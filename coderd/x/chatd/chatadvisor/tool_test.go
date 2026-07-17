package chatadvisor_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestAdvisorToolSuccess(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
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

func TestAdvisorToolPublishesAdviceDeltasWithToolCallID(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "Prefer "},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "the small diff."},
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
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	var published []codersdk.ChatMessagePart
	resp := runAdvisorToolWithPublisher(t, tool, chatadvisor.AdvisorArgs{Question: "What's safest?"},
		func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			require.Equal(t, codersdk.ChatMessageRoleTool, role)
			published = append(published, part)
		})
	require.False(t, resp.IsError)
	require.Len(t, published, 2)
	for _, part := range published {
		require.Equal(t, codersdk.ChatMessagePartTypeToolResult, part.Type)
		require.Equal(t, "call-1", part.ToolCallID)
		require.Equal(t, chatadvisor.ToolName, part.ToolName)
	}
	require.Equal(t, "Prefer ", published[0].ResultDelta)
	require.Equal(t, "the small diff.", published[1].ResultDelta)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "Prefer the small diff.", result.Advice)
}

func TestAdvisorToolPublishesAdviceResetWithToolCallID(t *testing.T) {
	t.Parallel()

	type publishedEvent struct {
		kind       string
		toolCallID string
		delta      string
	}
	var calls int

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

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	var published []publishedEvent
	resp := runAdvisorToolWithPublisher(t, tool, chatadvisor.AdvisorArgs{Question: "What's safest?"},
		func(role codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
			require.Equal(t, codersdk.ChatMessageRoleTool, role)
			require.Equal(t, codersdk.ChatMessagePartTypeToolResult, part.Type)
			require.Equal(t, chatadvisor.ToolName, part.ToolName)
			kind := "delta"
			if part.ResultReset {
				kind = "reset"
			}
			published = append(published, publishedEvent{
				kind:       kind,
				toolCallID: part.ToolCallID,
				delta:      part.ResultDelta,
			})
		})
	require.False(t, resp.IsError)
	require.Equal(t, []publishedEvent{
		{kind: "delta", toolCallID: "call-1", delta: "stale "},
		{kind: "reset", toolCallID: "call-1"},
		{kind: "delta", toolCallID: "call-1", delta: "fresh advice"},
	}, published)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeAdvice, result.Type)
	require.Equal(t, "fresh advice", result.Advice)
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

func TestAdvisorToolMissingPublisherReturnsError(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime: mustAdvisorRuntime(t),
		GetConversationSnapshot: func() []fantasy.Message {
			return nil
		},
	})

	data, err := json.Marshal(chatadvisor.AdvisorArgs{Question: "anything?"})
	require.NoError(t, err)

	resp, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "advisor",
		Input: string(data),
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "message-part publisher")
	require.Contains(t, resp.Content, "internal tool bug")
}

func TestAdvisorToolPassesNormalQuestion(t *testing.T) {
	t.Parallel()

	var capturedQuestion string
	tool := advisorToolCapturingQuestion(t, &capturedQuestion)

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "What's safest?"})
	require.False(t, resp.IsError)
	require.Equal(t, "What's safest?", capturedQuestion)
}

func TestAdvisorToolPreservesQuestionAtLimit(t *testing.T) {
	t.Parallel()

	var capturedQuestion string
	tool := advisorToolCapturingQuestion(t, &capturedQuestion)
	question := strings.Repeat("界", 2000)

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: question})
	require.False(t, resp.IsError)
	require.Equal(t, 2000, utf8.RuneCountInString(capturedQuestion))
	require.Equal(t, question, capturedQuestion)
}

func TestAdvisorToolTruncatesLongQuestion(t *testing.T) {
	t.Parallel()

	var capturedQuestion string
	tool := advisorToolCapturingQuestion(t, &capturedQuestion)
	longQuestion := strings.Repeat("界", 2001)

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: longQuestion})
	require.False(t, resp.IsError)
	require.True(t, utf8.ValidString(capturedQuestion))
	require.Equal(t, 2000, utf8.RuneCountInString(capturedQuestion))
	require.Equal(t, strings.Repeat("界", 2000), capturedQuestion)
}

func TestAdvisorToolInfoDocumentsQuestionLimit(t *testing.T) {
	t.Parallel()

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 mustAdvisorRuntime(t),
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	info := tool.Info()
	require.Contains(t, info.Description, "2000 runes")
	require.Contains(t, chatadvisor.ParentGuidanceBlock, "2000 runes")

	questionParam, ok := info.Parameters["question"].(map[string]any)
	require.True(t, ok)
	description, ok := questionParam["description"].(string)
	require.True(t, ok)
	require.Contains(t, description, "2000 runes")
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

func TestAdvisorToolReportsNestedError(t *testing.T) {
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

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "why?"})
	require.False(t, resp.IsError)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeError, result.Type)
	require.Contains(t, result.Error, "boom")
	require.Empty(t, result.Advice)
	require.Empty(t, result.AdvisorModel)
	// A failed nested run does not consume the per-run quota.
	require.Equal(t, 1, result.RemainingUses)
}

func TestAdvisorToolReportsLimitReached(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "first"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	first := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "first?"})
	require.False(t, first.IsError)

	second := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "second?"})
	require.False(t, second.IsError)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(second.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeLimitReached, result.Type)
	require.Equal(t, 0, result.RemainingUses)
	require.Empty(t, result.Advice)
	require.Empty(t, result.Error)
	require.Empty(t, result.AdvisorModel)
}

func TestAdvisorToolReportsEmptyModelOutput(t *testing.T) {
	t.Parallel()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	tool := chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})

	resp := runAdvisorTool(t, tool, chatadvisor.AdvisorArgs{Question: "anything?"})
	require.False(t, resp.IsError)

	var result chatadvisor.AdvisorResult
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	require.Equal(t, chatadvisor.ResultTypeError, result.Type)
	require.Contains(t, result.Error, "no text output")
	require.Empty(t, result.Advice)
	// An advisor call that produces no advice does not count as a
	// successful use, so the quota must still be available.
	require.Equal(t, 1, result.RemainingUses)
}

func mustAdvisorRuntime(t *testing.T) *chatadvisor.Runtime {
	t.Helper()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
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

func advisorToolCapturingQuestion(t *testing.T, capturedQuestion *string) fantasy.AgentTool {
	t.Helper()

	runtime, err := chatadvisor.NewRuntime(chatadvisor.RuntimeConfig{
		Model: &chattest.FakeModel{
			ProviderName: "test-provider",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
				require.NotEmpty(t, call.Prompt)
				*capturedQuestion = singleText(t, call.Prompt[len(call.Prompt)-1])
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "captured advice"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		},
		MaxUsesPerRun:   1,
		MaxOutputTokens: 64,
	})
	require.NoError(t, err)

	return chatadvisor.Tool(chatadvisor.ToolOptions{
		Runtime:                 runtime,
		GetConversationSnapshot: func() []fantasy.Message { return nil },
	})
}

func runAdvisorTool(
	t *testing.T,
	tool fantasy.AgentTool,
	args chatadvisor.AdvisorArgs,
) fantasy.ToolResponse {
	t.Helper()
	return runAdvisorToolWithPublisher(t, tool, args, func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {})
}

func runAdvisorToolWithPublisher(
	t *testing.T,
	tool fantasy.AgentTool,
	args chatadvisor.AdvisorArgs,
	publish func(codersdk.ChatMessageRole, codersdk.ChatMessagePart),
) fantasy.ToolResponse {
	t.Helper()

	data, err := json.Marshal(args)
	require.NoError(t, err)

	ctx := chatloop.WithMessagePartPublisher(t.Context(), publish)
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "advisor",
		Input: string(data),
	})
	require.NoError(t, err)
	return resp
}
