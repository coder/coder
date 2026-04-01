package chatdebug //nolint:testpackage // Uses unexported normalization helpers.

import (
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCall_PreservesToolSchemasAndMessageToolPayloads(t *testing.T) {
	t.Parallel()

	payload := normalizeCall(fantasy.Call{
		Prompt: fantasy.Prompt{
			{
				Role: fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{
					fantasy.ToolCallPart{
						ToolCallID: "call-search",
						ToolName:   "search_docs",
						Input:      `{"query":"debug panel"}`,
					},
				},
			},
			{
				Role: fantasy.MessageRoleTool,
				Content: []fantasy.MessagePart{
					fantasy.ToolResultPart{
						ToolCallID: "call-search",
						Output: fantasy.ToolResultOutputContentText{
							Text: `{"matches":["model.go","DebugStepCard.tsx"]}`,
						},
					},
				},
			},
		},
		Tools: []fantasy.Tool{
			fantasy.FunctionTool{
				Name:        "search_docs",
				Description: "Searches documentation.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []string{"query"},
				},
			},
		},
	})

	require.Len(t, payload.Tools, 1)
	require.True(t, payload.Tools[0].HasInputSchema)
	require.JSONEq(t, `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
		string(payload.Tools[0].InputSchema))

	require.Len(t, payload.Messages, 2)
	require.Equal(t, `{"query":"debug panel"}`, payload.Messages[0].Parts[0].Arguments)
	require.Equal(t,
		`{"matches":["model.go","DebugStepCard.tsx"]}`,
		payload.Messages[1].Parts[0].Result,
	)
}

func TestNormalizeResponse_PreservesToolCallArguments(t *testing.T) {
	t.Parallel()

	payload := normalizeResponse(&fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.ToolCallContent{
				ToolCallID: "call-calc",
				ToolName:   "calculator",
				Input:      `{"operation":"add","operands":[2,2]}`,
			},
		},
	})

	require.Len(t, payload.Content, 1)
	require.Equal(t, "call-calc", payload.Content[0].ToolCallID)
	require.Equal(t, "calculator", payload.Content[0].ToolName)
	require.JSONEq(t,
		`{"operation":"add","operands":[2,2]}`,
		payload.Content[0].Arguments,
	)
	require.Equal(t, len(`{"operation":"add","operands":[2,2]}`), payload.Content[0].InputLength)
}
