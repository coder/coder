package chatdebug //nolint:testpackage // Uses unexported normalization helpers.

import (
	"strings"
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

func TestNormalizers_SkipTypedNilInterfaceValues(t *testing.T) {
	t.Parallel()

	t.Run("MessageParts", func(t *testing.T) {
		t.Parallel()

		var nilPart *fantasy.TextPart
		parts := normalizeMessageParts([]fantasy.MessagePart{
			nilPart,
			fantasy.TextPart{Text: "hello"},
		})
		require.Len(t, parts, 1)
		require.Equal(t, "text", parts[0].Type)
		require.Equal(t, "hello", parts[0].Text)
	})

	t.Run("Tools", func(t *testing.T) {
		t.Parallel()

		var nilTool *fantasy.FunctionTool
		tools := normalizeTools([]fantasy.Tool{
			nilTool,
			fantasy.FunctionTool{Name: "search_docs"},
		})
		require.Len(t, tools, 1)
		require.Equal(t, "function", tools[0].Type)
		require.Equal(t, "search_docs", tools[0].Name)
	})

	t.Run("ContentParts", func(t *testing.T) {
		t.Parallel()

		var nilContent *fantasy.TextContent
		content := normalizeContentParts(fantasy.ResponseContent{
			nilContent,
			fantasy.TextContent{Text: "hello"},
		})
		require.Len(t, content, 1)
		require.Equal(t, "text", content[0].Type)
		require.Equal(t, "hello", content[0].Text)
	})
}

func TestAppendNormalizedStreamContent_PreservesOrderAndCanonicalTypes(t *testing.T) {
	t.Parallel()

	var content []normalizedContentPart
	streamDebugBytes := 0
	for _, part := range []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "before "},
		{Type: fantasy.StreamPartTypeToolCall, ID: "call-1", ToolCallName: "search_docs", ToolCallInput: `{"query":"debug"}`},
		{Type: fantasy.StreamPartTypeToolResult, ID: "call-1", ToolCallName: "search_docs", ToolCallInput: `{"matches":1}`},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "after"},
	} {
		content = appendNormalizedStreamContent(content, part, &streamDebugBytes)
	}

	require.Equal(t, []normalizedContentPart{
		{Type: "text", Text: "before "},
		{Type: "tool_call", ToolCallID: "call-1", ToolName: "search_docs", Arguments: `{"query":"debug"}`, InputLength: len(`{"query":"debug"}`)},
		{Type: "tool_result", ToolCallID: "call-1", ToolName: "search_docs", Result: `{"matches":1}`},
		{Type: "text", Text: "after"},
	}, content)
}

func TestAppendNormalizedStreamContent_GlobalTextCap(t *testing.T) {
	t.Parallel()

	streamDebugBytes := 0
	long := strings.Repeat("a", maxStreamDebugTextBytes)
	var content []normalizedContentPart
	for _, part := range []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextDelta, Delta: long},
		{Type: fantasy.StreamPartTypeToolCall, ID: "call-1", ToolCallName: "search_docs", ToolCallInput: `{}`},
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "tail"},
	} {
		content = appendNormalizedStreamContent(content, part, &streamDebugBytes)
	}

	require.Len(t, content, 2)
	require.Equal(t, strings.Repeat("a", maxStreamDebugTextBytes), content[0].Text)
	require.Equal(t, "tool_call", content[1].Type)
	require.Equal(t, maxStreamDebugTextBytes, streamDebugBytes)
}

func TestBoundText_RespectsDocumentedRuneLimit(t *testing.T) {
	t.Parallel()

	runes := make([]rune, MaxMessagePartTextLength+5)
	for i := range runes {
		runes[i] = 'a'
	}
	input := string(runes)
	got := boundText(input)
	require.Equal(t, MaxMessagePartTextLength, len([]rune(got)))
	require.Equal(t, '…', []rune(got)[len([]rune(got))-1])
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
