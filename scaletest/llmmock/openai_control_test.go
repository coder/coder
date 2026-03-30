package llmmock

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/chatcontrol"
)

func TestBuildOpenAIResponseWithoutSentinel(t *testing.T) {
	t.Parallel()

	resp, err := buildOpenAIResponse(llmRequest{Model: "scaletest-model"}, uuid.MustParse("11111111-1111-1111-1111-111111111111"), time.Unix(1, 0), 0)
	require.NoError(t, err)
	require.Equal(t, "stop", resp.Choices[0].FinishReason)
	require.Empty(t, resp.Choices[0].Message.ToolCalls)
	require.Equal(t, "This is a mock response from OpenAI.", resp.Choices[0].Message.Content)
}

func TestBuildOpenAIResponseEmitsToolCall(t *testing.T) {
	t.Parallel()

	prompt, err := chatcontrol.PrefixPrompt("Reply with one short sentence.", chatcontrol.Control{
		ToolCallsThisTurn: 1,
	})
	require.NoError(t, err)

	resp, err := buildOpenAIResponse(llmRequest{
		Model: "scaletest-model",
		Messages: []openAIMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Tools: []openAITool{{
			Type: "function",
			Function: openAIToolFunction{
				Name: chatcontrol.DefaultToolName,
			},
		}},
	}, uuid.MustParse("22222222-2222-2222-2222-222222222222"), time.Unix(2, 0), 0)
	require.NoError(t, err)
	require.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, chatcontrol.DefaultToolName, resp.Choices[0].Message.ToolCalls[0].Function.Name)
	require.Contains(t, resp.Choices[0].Message.ToolCalls[0].Function.Arguments, chatcontrol.DefaultToolCommand)
}

func TestBuildOpenAIResponseSettlesAfterPlannedToolCalls(t *testing.T) {
	t.Parallel()

	prompt, err := chatcontrol.PrefixPrompt("Continue.", chatcontrol.Control{
		ToolCallsThisTurn: 1,
	})
	require.NoError(t, err)

	resp, err := buildOpenAIResponse(llmRequest{
		Model: "scaletest-model",
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
			{Role: "assistant", ToolCalls: []openAIToolCall{{ID: "call_1", Type: "function", Function: openAIToolCallFunction{Name: chatcontrol.DefaultToolName, Arguments: `{"command":"echo scaletest"}`}}}},
			{Role: "tool", ToolCallID: "call_1", Content: "ok"},
		},
		Tools: []openAITool{{
			Type:     "function",
			Function: openAIToolFunction{Name: chatcontrol.DefaultToolName},
		}},
	}, uuid.MustParse("33333333-3333-3333-3333-333333333333"), time.Unix(3, 0), 0)
	require.NoError(t, err)
	require.Equal(t, "stop", resp.Choices[0].FinishReason)
	require.Empty(t, resp.Choices[0].Message.ToolCalls)
	require.NotEmpty(t, resp.Choices[0].Message.Content)
}

func TestBuildOpenAIResponseRejectsMissingTool(t *testing.T) {
	t.Parallel()

	prompt, err := chatcontrol.PrefixPrompt("Continue.", chatcontrol.Control{
		ToolCallsThisTurn: 1,
	})
	require.NoError(t, err)

	_, err = buildOpenAIResponse(llmRequest{
		Model: "scaletest-model",
		Messages: []openAIMessage{{
			Role:    "user",
			Content: prompt,
		}},
	}, uuid.MustParse("44444444-4444-4444-4444-444444444444"), time.Unix(4, 0), 0)
	require.ErrorContains(t, err, "requested tool")
}

func TestSendOpenAIStreamIncludesToolCalls(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Unix(5, 0).Unix(),
		Model:   "scaletest-model",
		Choices: []openAIResponseChoice{{
			Index: 0,
			Message: openAIMessage{
				Role: "assistant",
				ToolCalls: []openAIToolCall{{
					Index: 0,
					ID:    "call_test",
					Type:  "function",
					Function: openAIToolCallFunction{
						Name:      chatcontrol.DefaultToolName,
						Arguments: `{"command":"echo scaletest"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}

	srv.sendOpenAIStream(t.Context(), writer, resp)
	body := writer.Body.String()
	require.Contains(t, body, "tool_calls")
	require.Contains(t, body, "\"finish_reason\":\"tool_calls\"")
	require.Contains(t, body, "[DONE]")
}
