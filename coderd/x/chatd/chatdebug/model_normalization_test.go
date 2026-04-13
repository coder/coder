package chatdebug //nolint:testpackage // Uses unexported normalization helpers.

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
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
	require.Equal(t, "tool-call", payload.Messages[0].Parts[0].Type)
	require.Equal(t, `{"query":"debug panel"}`, payload.Messages[0].Parts[0].Arguments)
	require.Equal(t, "tool-result", payload.Messages[1].Parts[0].Type)
	require.Equal(t,
		`{"matches":["model.go","DebugStepCard.tsx"]}`,
		payload.Messages[1].Parts[0].Result,
	)
}

func TestNormalizeTools_PreservesExecutableProviderToolID(t *testing.T) {
	t.Parallel()

	pdt := fantasy.ProviderDefinedTool{
		ID:   "anthropic.computer_use",
		Name: "computer",
	}
	ept := fantasy.NewExecutableProviderTool(pdt, func(context.Context, fantasy.ToolCall) (fantasy.ToolResponse, error) {
		return fantasy.ToolResponse{}, nil
	})

	tools := normalizeTools([]fantasy.Tool{ept})
	require.Len(t, tools, 1)
	require.Equal(t, "anthropic.computer_use", tools[0].ID)
	require.Equal(t, "computer", tools[0].Name)
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
		{Type: "tool-call", ToolCallID: "call-1", ToolName: "search_docs", Arguments: `{"query":"debug"}`, InputLength: utf8.RuneCountInString(`{"query":"debug"}`)},
		{Type: "tool-result", ToolCallID: "call-1", ToolName: "search_docs", Result: `{"matches":1}`},
		{Type: "text", Text: "after"},
	}, content)
}

func TestAppendNormalizedStreamContent_ToolInputAttributionPerCall(t *testing.T) {
	t.Parallel()

	var content []normalizedContentPart
	streamDebugBytes := 0
	for _, part := range []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeToolInputStart, ID: "call-a", ToolCallName: "search", Delta: `{"q`},
		{Type: fantasy.StreamPartTypeToolInputDelta, ID: "call-a", ToolCallName: "search", Delta: `uery`},
		// Interleaved second tool call.
		{Type: fantasy.StreamPartTypeToolInputStart, ID: "call-b", ToolCallName: "calc", Delta: `{"op`},
		{Type: fantasy.StreamPartTypeToolInputDelta, ID: "call-a", ToolCallName: "search", Delta: `":"x"}`},
		{Type: fantasy.StreamPartTypeToolInputEnd, ID: "call-b", ToolCallName: "calc", Delta: `":"add"}`},
	} {
		content = appendNormalizedStreamContent(content, part, &streamDebugBytes)
	}

	require.Equal(t, []normalizedContentPart{
		{Type: "tool_input", ToolCallID: "call-a", ToolName: "search", Arguments: `{"query":"x"}`},
		{Type: "tool_input", ToolCallID: "call-b", ToolName: "calc", Arguments: `{"op":"add"}`},
	}, content)
}

func TestAppendNormalizedStreamContent_ToolInputAcrossInterleavedText(t *testing.T) {
	t.Parallel()

	var content []normalizedContentPart
	streamDebugBytes := 0
	for _, part := range []fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeToolInputStart, ID: "call-a", ToolCallName: "search", Delta: `{"q`},
		// Text delta interleaved between tool_input deltas for call-a.
		{Type: fantasy.StreamPartTypeTextDelta, Delta: "thinking..."},
		{Type: fantasy.StreamPartTypeToolInputDelta, ID: "call-a", ToolCallName: "search", Delta: `uery":"x"}`},
	} {
		content = appendNormalizedStreamContent(content, part, &streamDebugBytes)
	}

	require.Equal(t, []normalizedContentPart{
		{Type: "tool_input", ToolCallID: "call-a", ToolName: "search", Arguments: `{"query":"x"}`},
		{Type: "text", Text: "thinking..."},
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
	require.Equal(t, "tool-call", content[1].Type)
	require.Equal(t, maxStreamDebugTextBytes, streamDebugBytes)
}

func TestWrapStreamSeq_SourceCountExcludesToolResults(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}
	seq := wrapStreamSeq(context.Background(), handle, partsToSeq([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeToolResult, ID: "tool-1", ToolCallName: "search_docs"},
		{Type: fantasy.StreamPartTypeSource, ID: "source-1", URL: "https://example.com", Title: "docs"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
	}))

	partCount := 0
	for range seq {
		partCount++
	}
	require.Equal(t, 3, partCount)

	metadata, ok := handle.metadata.(map[string]any)
	require.True(t, ok)
	summary, ok := metadata["stream_summary"].(streamSummary)
	require.True(t, ok)
	require.Equal(t, 1, summary.SourceCount)
}

func TestWrapObjectStreamSeq_UsesStructuredOutputPayload(t *testing.T) {
	t.Parallel()

	handle := &stepHandle{
		stepCtx: &StepContext{StepID: uuid.New(), RunID: uuid.New(), ChatID: uuid.New()},
		sink:    &attemptSink{},
	}
	usage := fantasy.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	seq := wrapObjectStreamSeq(context.Background(), handle, objectPartsToSeq([]fantasy.ObjectStreamPart{
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ob"},
		{Type: fantasy.ObjectStreamPartTypeTextDelta, Delta: "ject"},
		{Type: fantasy.ObjectStreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: usage},
	}))

	partCount := 0
	for range seq {
		partCount++
	}
	require.Equal(t, 3, partCount)

	resp, ok := handle.response.(normalizedObjectResponsePayload)
	require.True(t, ok)
	require.Equal(t, normalizedObjectResponsePayload{
		RawTextLength:    utf8.RuneCountInString("object"),
		FinishReason:     string(fantasy.FinishReasonStop),
		Usage:            normalizeUsage(usage),
		StructuredOutput: true,
	}, resp)
}

func TestNormalizeResponse_UsesCanonicalToolTypes(t *testing.T) {
	t.Parallel()

	payload := normalizeResponse(&fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.ToolCallContent{
				ToolCallID: "call-calc",
				ToolName:   "calculator",
				Input:      `{"operation":"add","operands":[2,2]}`,
			},
			fantasy.ToolResultContent{
				ToolCallID: "call-calc",
				ToolName:   "calculator",
				Result:     fantasy.ToolResultOutputContentText{Text: `{"sum":4}`},
			},
		},
	})

	require.Len(t, payload.Content, 2)
	require.Equal(t, "tool-call", payload.Content[0].Type)
	require.Equal(t, "tool-result", payload.Content[1].Type)
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

func TestNormalizeToolResultOutput(t *testing.T) {
	t.Parallel()

	t.Run("TextValue", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentText{Text: "hello"})
		require.Equal(t, "hello", got)
	})

	t.Run("TextPointer", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(&fantasy.ToolResultOutputContentText{Text: "hello"})
		require.Equal(t, "hello", got)
	})

	t.Run("TextPointerNil", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput((*fantasy.ToolResultOutputContentText)(nil))
		require.Equal(t, "", got)
	})

	t.Run("ErrorValue", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentError{
			Error: xerrors.New("tool failed"),
		})
		require.Equal(t, "tool failed", got)
	})

	t.Run("ErrorValueNilError", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentError{Error: nil})
		require.Equal(t, "", got)
	})

	t.Run("ErrorPointer", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(&fantasy.ToolResultOutputContentError{
			Error: xerrors.New("ptr fail"),
		})
		require.Equal(t, "ptr fail", got)
	})

	t.Run("ErrorPointerNil", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput((*fantasy.ToolResultOutputContentError)(nil))
		require.Equal(t, "", got)
	})

	t.Run("ErrorPointerNilError", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(&fantasy.ToolResultOutputContentError{Error: nil})
		require.Equal(t, "", got)
	})

	t.Run("MediaWithText", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentMedia{
			Text:      "caption",
			MediaType: "image/png",
		})
		require.Equal(t, "caption", got)
	})

	t.Run("MediaWithoutText", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentMedia{
			MediaType: "image/png",
		})
		require.Equal(t, "[media output: image/png]", got)
	})

	t.Run("MediaWithoutTextOrType", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentMedia{})
		require.Equal(t, "[media output]", got)
	})

	t.Run("MediaPointerNil", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput((*fantasy.ToolResultOutputContentMedia)(nil))
		require.Equal(t, "", got)
	})

	t.Run("MediaPointerWithText", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(&fantasy.ToolResultOutputContentMedia{
			Text:      "ptr caption",
			MediaType: "image/jpeg",
		})
		require.Equal(t, "ptr caption", got)
	})

	t.Run("NilOutput", func(t *testing.T) {
		t.Parallel()
		got := normalizeToolResultOutput(nil)
		require.Equal(t, "", got)
	})

	t.Run("DefaultJSON", func(t *testing.T) {
		t.Parallel()
		// An unexpected type falls through to the default JSON
		// marshal branch.
		got := normalizeToolResultOutput(fantasy.ToolResultOutputContentText{
			Text: "fallback",
		})
		require.Equal(t, "fallback", got)
	})
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
	require.Equal(t, utf8.RuneCountInString(`{"operation":"add","operands":[2,2]}`), payload.Content[0].InputLength)
}
