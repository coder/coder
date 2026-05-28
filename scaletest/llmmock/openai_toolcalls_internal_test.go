package llmmock

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestBuildOpenAIChoiceTextOnlyDoesNotRequireTools(t *testing.T) {
	t.Parallel()

	srv := &Server{toolCallsPerTurn: 0}
	choice, err := srv.buildOpenAIChoice(openAIExecuteRequest(0))
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, choice.FinishReason)
	require.Empty(t, choice.Message.ToolCalls)
	require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
}

func TestBuildOpenAIChoiceNoUserMessageReturnsText(t *testing.T) {
	t.Parallel()

	srv := &Server{toolCallsPerTurn: 1, toolCallCommand: DefaultToolCallCommand}
	choice, err := srv.buildOpenAIChoice(llmRequest{
		Model:    "scaletest-model",
		Messages: []openAIMessage{{Role: "system", Content: "Reply with one short sentence."}},
	})
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, choice.FinishReason)
	require.Empty(t, choice.Message.ToolCalls)
	require.Equal(t, openAIDefaultResponseText, choice.Message.Content)
}

func TestBuildOpenAIChoiceEmitsFixedToolCallsUntilCompleted(t *testing.T) {
	t.Parallel()

	srv := &Server{toolCallsPerTurn: 2, toolCallCommand: DefaultToolCallCommand}
	for _, tt := range []struct {
		name          string
		completed     int
		wantFinish    string
		wantToolCalls int
	}{
		{name: "none completed", completed: 0, wantFinish: openAIToolCallFinishReason, wantToolCalls: 1},
		{name: "one completed", completed: 1, wantFinish: openAIToolCallFinishReason, wantToolCalls: 1},
		{name: "all completed", completed: 2, wantFinish: openAIStopFinishReason},
	} {
		t.Run(tt.name, func(t *testing.T) {
			choice, err := srv.buildOpenAIChoice(openAIExecuteRequest(tt.completed))
			require.NoError(t, err)
			require.Equal(t, 0, choice.Index)
			require.Equal(t, tt.wantFinish, choice.FinishReason)
			require.Len(t, choice.Message.ToolCalls, tt.wantToolCalls)

			if tt.wantToolCalls == 0 {
				require.NotEmpty(t, choice.Message.Content)
				return
			}
			if tt.completed == 0 {
				toolCall := choice.Message.ToolCalls[0]
				require.Equal(t, 0, toolCall.Index)
				require.True(t, strings.HasPrefix(toolCall.ID, "call_"))
				require.Equal(t, executeToolName, toolCall.Function.Name)
				require.JSONEq(t, `{"command":"echo scaletest"}`, toolCall.Function.Arguments)
			}
		})
	}
}

func TestBuildOpenAIChoiceRequiresExecuteToolWhenToolCallNeeded(t *testing.T) {
	t.Parallel()

	srv := &Server{toolCallsPerTurn: 1, toolCallCommand: DefaultToolCallCommand}
	req := openAIExecuteRequest(0)
	req.Tools = nil
	_, err := srv.buildOpenAIChoice(req)
	require.ErrorContains(t, err, `requested tool "execute" not present`)
}

func TestSendOpenAIStreamIncludesToolCalls(t *testing.T) {
	t.Parallel()

	toolCall := executeToolCall(DefaultToolCallCommand)

	srv := &Server{}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 7,
		Model:   "scaletest-model",
		Choices: []openAIResponseChoice{{
			Index:        0,
			Message:      openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{toolCall}},
			FinishReason: openAIToolCallFinishReason,
		}},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	srv.sendOpenAIStream(ctx, writer, resp)
	body := writer.Body.String()
	require.Contains(t, body, `"delta":{"role":"assistant","tool_calls"`)
	require.Contains(t, body, `"name":"execute"`)
	require.Contains(t, body, `"index":0`)
	require.Contains(t, body, `"finish_reason":"tool_calls"`)
	require.Equal(t, 2, strings.Count(body, "data: {"))
	require.Contains(t, body, "[DONE]")
}

func openAIExecuteRequest(completedToolCalls int) llmRequest {
	req := llmRequest{
		Model:    "scaletest-model",
		Messages: []openAIMessage{{Role: "user", Content: "Reply with one short sentence."}},
		Tools:    []openAITool{{Type: "function", Function: openAIToolFunction{Name: executeToolName}}},
	}
	for range completedToolCalls {
		req.Messages = append(req.Messages,
			openAIMessage{Role: "assistant", ToolCalls: []openAIToolCall{{ID: "call_done"}}},
			openAIMessage{Role: "tool", ToolCallID: "call_done", Content: "ok"},
		)
	}
	return req
}
