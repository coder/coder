package llmmock

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

const testToolCallCommand = "echo scaletest"

func TestBuildOpenAIChoice(t *testing.T) {
	t.Parallel()

	t.Run("TextOnlyDoesNotRequireTools", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 0}
		req := openAIExecuteRequest(0)
		req.Tools = nil
		choice := srv.buildOpenAIChoice(req)
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("NoUserMessageReturnsText", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 1, toolCallCommand: testToolCallCommand}
		choice := srv.buildOpenAIChoice(llmRequest{
			Model:    "scaletest-model",
			Messages: []openAIMessage{{Role: "system", Content: "Reply with one short sentence."}},
		})
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("EmitsToolCallNoneCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: testToolCallCommand}
		choice := srv.buildOpenAIChoice(openAIExecuteRequest(0))
		require.Equal(t, openAIToolCallFinishReason, choice.FinishReason)
		require.Len(t, choice.Message.ToolCalls, 1)
		toolCall := choice.Message.ToolCalls[0]
		requireExecuteToolCall(t, toolCall, testToolCallCommand)

		toolCallJSON, err := json.Marshal(toolCall)
		require.NoError(t, err)
		var toolCallFields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(toolCallJSON, &toolCallFields))
		require.NotContains(t, toolCallFields, "index")
	})

	t.Run("EmitsToolCallOneCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: testToolCallCommand}
		choice := srv.buildOpenAIChoice(openAIExecuteRequest(1))
		require.Equal(t, openAIToolCallFinishReason, choice.FinishReason)
		require.Len(t, choice.Message.ToolCalls, 1)
		requireExecuteToolCall(t, choice.Message.ToolCalls[0], testToolCallCommand)
	})

	t.Run("StopsAfterAllToolCallsCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: testToolCallCommand}
		choice := srv.buildOpenAIChoice(openAIExecuteRequest(2))
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("FallsBackToTextWhenExecuteToolMissing", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 1, toolCallCommand: testToolCallCommand}
		req := openAIExecuteRequest(0)
		req.Tools = nil
		choice := srv.buildOpenAIChoice(req)
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})
}

func TestSendOpenAIStreamIncludesToolCalls(t *testing.T) {
	t.Parallel()

	toolCall := executeToolCall(testToolCallCommand)

	srv := &Server{}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 7,
		Model:   "scaletest-model",
		Choices: []openAIResponseChoice{{
			Message:      openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{toolCall}},
			FinishReason: openAIToolCallFinishReason,
		}},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	srv.sendOpenAIStream(ctx, writer, resp)
	events := sseDataEvents(t, writer.Body.String())
	require.Len(t, events, 3)

	first := decodeStreamChunk(t, events[0])
	require.Len(t, first.Choices, 1)
	require.Nil(t, first.Choices[0].FinishReason)
	require.Equal(t, "assistant", first.Choices[0].Delta.Role)
	require.Len(t, first.Choices[0].Delta.ToolCalls, 1)
	requireExecuteStreamToolCall(t, events[0], first.Choices[0].Delta.ToolCalls[0], testToolCallCommand)

	second := decodeStreamChunk(t, events[1])
	require.Len(t, second.Choices, 1)
	require.NotNil(t, second.Choices[0].FinishReason)
	require.Equal(t, openAIToolCallFinishReason, *second.Choices[0].FinishReason)
	require.Empty(t, second.Choices[0].Delta.ToolCalls)

	require.Equal(t, "[DONE]", events[2])
}

func decodeStreamChunk(t *testing.T, data string) openAIStreamChunk {
	t.Helper()

	var chunk openAIStreamChunk
	require.NoError(t, json.Unmarshal([]byte(data), &chunk))
	return chunk
}

func sseDataEvents(t *testing.T, body string) []string {
	t.Helper()

	var events []string
	for _, event := range strings.Split(body, "\n\n") {
		if event == "" {
			continue
		}

		var dataLines []string
		for _, line := range strings.Split(event, "\n") {
			data, ok := strings.CutPrefix(line, "data: ")
			if ok {
				dataLines = append(dataLines, data)
			}
		}
		if len(dataLines) > 0 {
			events = append(events, strings.Join(dataLines, "\n"))
		}
	}
	return events
}

// requireExecuteStreamToolCall asserts the streamed tool-call delta and that
// its JSON payload includes the index field, which the production type marks
// as non-omitempty but is otherwise indistinguishable from a zero default in
// the decoded struct.
func requireExecuteStreamToolCall(t *testing.T, rawEvent string, toolCall openAIToolCallDelta, command string) {
	t.Helper()

	require.Zero(t, toolCall.Index)

	var rawChunk struct {
		Choices []struct {
			Delta struct {
				ToolCalls []map[string]json.RawMessage `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal([]byte(rawEvent), &rawChunk))
	require.Len(t, rawChunk.Choices, 1)
	require.Len(t, rawChunk.Choices[0].Delta.ToolCalls, 1)
	require.Contains(t, rawChunk.Choices[0].Delta.ToolCalls[0], "index")

	requireExecuteToolCall(t, openAIToolCall{
		ID:       toolCall.ID,
		Type:     toolCall.Type,
		Function: toolCall.Function,
	}, command)
}

func requireExecuteToolCall(t *testing.T, toolCall openAIToolCall, command string) {
	t.Helper()

	require.True(t, strings.HasPrefix(toolCall.ID, "call_"), "tool call ID %q", toolCall.ID)
	require.Equal(t, "function", toolCall.Type)
	require.Equal(t, executeToolName, toolCall.Function.Name)
	expectedPayload, err := json.Marshal(map[string]string{"command": command})
	require.NoError(t, err)
	require.JSONEq(t, string(expectedPayload), toolCall.Function.Arguments)
}

func openAIExecuteRequest(completedToolCalls int) llmRequest {
	req := llmRequest{
		Model:    "scaletest-model",
		Messages: []openAIMessage{{Role: "user", Content: "Reply with one short sentence."}},
		Tools:    []openAITool{{Type: "function", Function: openAIToolFunction{Name: executeToolName}}},
	}
	for i := range completedToolCalls {
		toolCall := executeToolCall(testToolCallCommand)
		toolCall.ID = fmt.Sprintf("call_done_%d", i)
		req.Messages = append(req.Messages,
			openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{toolCall}},
			openAIMessage{Role: "tool", ToolCallID: toolCall.ID, Content: "ok"},
		)
	}
	return req
}
