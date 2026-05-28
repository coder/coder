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

func TestBuildOpenAIChoice(t *testing.T) {
	t.Parallel()

	t.Run("TextOnlyDoesNotRequireTools", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 0}
		req := openAIExecuteRequest(0)
		req.Tools = nil
		choice, err := srv.buildOpenAIChoice(req)
		require.NoError(t, err)
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("NoUserMessageReturnsText", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 1, toolCallCommand: defaultToolCallCommand}
		choice, err := srv.buildOpenAIChoice(llmRequest{
			Model:    "scaletest-model",
			Messages: []openAIMessage{{Role: "system", Content: "Reply with one short sentence."}},
		})
		require.NoError(t, err)
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("EmitsToolCallNoneCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: defaultToolCallCommand}
		choice, err := srv.buildOpenAIChoice(openAIExecuteRequest(0))
		require.NoError(t, err)
		require.Equal(t, openAIToolCallFinishReason, choice.FinishReason)
		require.Len(t, choice.Message.ToolCalls, 1)
		toolCall := choice.Message.ToolCalls[0]
		requireExecuteToolCall(t, toolCall, defaultToolCallCommand)

		toolCallJSON, err := json.Marshal(toolCall)
		require.NoError(t, err)
		var toolCallFields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(toolCallJSON, &toolCallFields))
		require.NotContains(t, toolCallFields, "index")
	})

	t.Run("EmitsToolCallOneCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: defaultToolCallCommand}
		choice, err := srv.buildOpenAIChoice(openAIExecuteRequest(1))
		require.NoError(t, err)
		require.Equal(t, openAIToolCallFinishReason, choice.FinishReason)
		require.Len(t, choice.Message.ToolCalls, 1)
		requireExecuteToolCall(t, choice.Message.ToolCalls[0], defaultToolCallCommand)
	})

	t.Run("StopsAfterAllToolCallsCompleted", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 2, toolCallCommand: defaultToolCallCommand}
		choice, err := srv.buildOpenAIChoice(openAIExecuteRequest(2))
		require.NoError(t, err)
		require.Equal(t, openAIStopFinishReason, choice.FinishReason)
		require.Empty(t, choice.Message.ToolCalls)
		require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
	})

	t.Run("RequiresExecuteToolWhenToolCallNeeded", func(t *testing.T) {
		t.Parallel()

		srv := &Server{toolCallsPerTurn: 1, toolCallCommand: defaultToolCallCommand}
		req := openAIExecuteRequest(0)
		req.Tools = nil
		_, err := srv.buildOpenAIChoice(req)
		require.ErrorContains(t, err, "requested tool \"execute\" not present")
	})
}

func TestSendOpenAIStreamIncludesToolCalls(t *testing.T) {
	t.Parallel()

	toolCall := executeToolCall(defaultToolCallCommand)

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

	first := decodeOpenAIStreamEvent(t, events[0])
	require.Len(t, first.Choices, 1)
	require.Nil(t, first.Choices[0].FinishReason)
	require.Equal(t, "assistant", first.Choices[0].Delta.Role)
	require.Len(t, first.Choices[0].Delta.ToolCalls, 1)
	requireExecuteStreamToolCall(t, first.Choices[0].Delta.ToolCalls[0], defaultToolCallCommand)

	second := decodeOpenAIStreamEvent(t, events[1])
	require.Len(t, second.Choices, 1)
	require.NotNil(t, second.Choices[0].FinishReason)
	require.Equal(t, openAIToolCallFinishReason, *second.Choices[0].FinishReason)
	require.Empty(t, second.Choices[0].Delta.ToolCalls)

	require.Equal(t, "[DONE]", events[2])
}

type openAIStreamEvent struct {
	Choices []struct {
		Delta struct {
			Role      string                 `json:"role"`
			ToolCalls []openAIStreamToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIStreamToolCall struct {
	Index    *int                   `json:"index"`
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIToolCallFunction `json:"function"`
}

func decodeOpenAIStreamEvent(t *testing.T, data string) openAIStreamEvent {
	t.Helper()

	var event openAIStreamEvent
	require.NoError(t, json.Unmarshal([]byte(data), &event))
	return event
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

func requireExecuteStreamToolCall(t *testing.T, toolCall openAIStreamToolCall, command string) {
	t.Helper()

	require.NotNil(t, toolCall.Index)
	require.Zero(t, *toolCall.Index)
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
		toolCall := executeToolCall(defaultToolCallCommand)
		toolCall.ID = fmt.Sprintf("call_done_%d", i)
		req.Messages = append(req.Messages,
			openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{toolCall}},
			openAIMessage{Role: "tool", ToolCallID: toolCall.ID, Content: "ok"},
		)
	}
	return req
}
