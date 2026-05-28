package llmmock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestBuildOpenAIResponseTextOnlyWithoutTools(t *testing.T) {
	t.Parallel()

	resp, err := buildOpenAIResponse(
		llmRequest{Model: "scaletest-model"},
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		time.Unix(1, 0),
		0,
		"",
		toolCallConfig{MaxToolCallsPerTurn: 0},
	)
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, resp.Choices[0].FinishReason)
	require.Empty(t, resp.Choices[0].Message.ToolCalls)
	require.Equal(t, openAIDefaultResponseText, resp.Choices[0].Message.Content)
}

func TestBuildOpenAIResponseTextOnlyWithoutUserMessage(t *testing.T) {
	t.Parallel()

	resp, err := buildOpenAIResponse(
		llmRequest{Model: "scaletest-model"},
		uuid.MustParse("12121212-1212-1212-1212-121212121212"),
		time.Unix(12, 0),
		0,
		"chat-a",
		toolCallConfig{
			MinToolCallsPerTurn: 1,
			MaxToolCallsPerTurn: 1,
			ToolCallCommand:     defaultToolCallCommand,
			Seed:                123,
		},
	)
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, resp.Choices[0].FinishReason)
	require.Empty(t, resp.Choices[0].Message.ToolCalls)
	require.Equal(t, openAIDefaultResponseText, resp.Choices[0].Message.Content)
}

func TestBuildOpenAIResponseFixedToolCallTurn(t *testing.T) {
	t.Parallel()

	cfg := toolCallConfig{
		MinToolCallsPerTurn: 2,
		MaxToolCallsPerTurn: 2,
		ToolCallCommand:     defaultToolCallCommand,
		Seed:                123,
	}
	baseReq := llmRequest{
		Model: "scaletest-model",
		Messages: []openAIMessage{{
			Role:    "user",
			Content: "Reply with one short sentence.",
		}},
		Tools: []openAITool{{
			Type:     "function",
			Function: openAIToolFunction{Name: executeToolName},
		}},
	}

	resp, err := buildOpenAIResponse(baseReq, uuid.MustParse("22222222-2222-2222-2222-222222222222"), time.Unix(2, 0), 0, "chat-a", cfg)
	require.NoError(t, err)
	require.Equal(t, openAIToolCallFinishReason, resp.Choices[0].FinishReason)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	require.Equal(t, 0, resp.Choices[0].Message.ToolCalls[0].Index)
	require.True(t, strings.HasPrefix(resp.Choices[0].Message.ToolCalls[0].ID, "call_"))
	require.Equal(t, executeToolName, resp.Choices[0].Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"command":"echo scaletest"}`, resp.Choices[0].Message.ToolCalls[0].Function.Arguments)

	oneCompleted := baseReq
	oneCompleted.Messages = append(oneCompleted.Messages,
		openAIMessage{Role: "assistant", ToolCalls: resp.Choices[0].Message.ToolCalls},
		openAIMessage{Role: "tool", ToolCallID: resp.Choices[0].Message.ToolCalls[0].ID, Content: "ok"},
	)
	resp, err = buildOpenAIResponse(oneCompleted, uuid.MustParse("33333333-3333-3333-3333-333333333333"), time.Unix(3, 0), 0, "chat-a", cfg)
	require.NoError(t, err)
	require.Equal(t, openAIToolCallFinishReason, resp.Choices[0].FinishReason)

	twoCompleted := oneCompleted
	twoCompleted.Messages = append(twoCompleted.Messages,
		openAIMessage{Role: "assistant", ToolCalls: resp.Choices[0].Message.ToolCalls},
		openAIMessage{Role: "tool", ToolCallID: resp.Choices[0].Message.ToolCalls[0].ID, Content: "ok"},
	)
	resp, err = buildOpenAIResponse(twoCompleted, uuid.MustParse("44444444-4444-4444-4444-444444444444"), time.Unix(4, 0), 0, "chat-a", cfg)
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, resp.Choices[0].FinishReason)
	require.Empty(t, resp.Choices[0].Message.ToolCalls)
	require.NotEmpty(t, resp.Choices[0].Message.Content)
}

func TestTargetForTurnDeterministicWithinRange(t *testing.T) {
	t.Parallel()

	first := targetForTurn(123, "chat-a", 0, 2, 6)
	require.Equal(t, first, targetForTurn(123, "chat-a", 0, 2, 6))
	require.GreaterOrEqual(t, first, 2)
	require.LessOrEqual(t, first, 6)

	seed := seedForTurn(123, "chat-a", 0)
	require.Equal(t, seed, seedForTurn(123, "chat-a", 0))
	require.NotEqual(t, seed, seedForTurn(123, "chat-a", 1))
	require.NotEqual(t, seed, seedForTurn(123, "chat-b", 0))
}

func TestBuildOpenAIResponseRejectsMissingToolOnlyWhenNeeded(t *testing.T) {
	t.Parallel()

	req := llmRequest{
		Model: "scaletest-model",
		Messages: []openAIMessage{{
			Role:    "user",
			Content: "Continue.",
		}},
	}
	cfg := toolCallConfig{
		MinToolCallsPerTurn: 1,
		MaxToolCallsPerTurn: 1,
		ToolCallCommand:     defaultToolCallCommand,
		Seed:                456,
	}
	_, err := buildOpenAIResponse(req, uuid.MustParse("55555555-5555-5555-5555-555555555555"), time.Unix(5, 0), 0, "chat-a", cfg)
	require.ErrorContains(t, err, "requested tool")

	cfg.MaxToolCallsPerTurn = 0
	resp, err := buildOpenAIResponse(req, uuid.MustParse("66666666-6666-6666-6666-666666666666"), time.Unix(6, 0), 0, "chat-a", cfg)
	require.NoError(t, err)
	require.Equal(t, openAIStopFinishReason, resp.Choices[0].FinishReason)
}

func TestHandleOpenAIUsesChatIDHeaderForToolCallTarget(t *testing.T) {
	t.Parallel()

	const seed = uint64(1)
	require.Equal(t, 0, targetForTurn(seed, "", 0, 0, 1))
	require.Equal(t, 1, targetForTurn(seed, "chat-a", 0, 0, 1))

	srv := &Server{
		toolCallConfig: toolCallConfig{
			MinToolCallsPerTurn: 0,
			MaxToolCallsPerTurn: 1,
			ToolCallCommand:     defaultToolCallCommand,
			Seed:                seed,
		},
	}

	call := func(t *testing.T, chatID string) openAIResponse {
		t.Helper()

		body := `{"model":"scaletest-model","messages":[{"role":"user","content":"Continue."}],"tools":[{"type":"function","function":{"name":"execute"}}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		if chatID != "" {
			req.Header.Set(coderChatIDHeader, chatID)
		}
		writer := httptest.NewRecorder()
		srv.handleOpenAIWithLabels(writer, req)
		require.Equal(t, http.StatusOK, writer.Code)

		var resp openAIResponse
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &resp))
		return resp
	}

	withoutHeader := call(t, "")
	require.Equal(t, openAIStopFinishReason, withoutHeader.Choices[0].FinishReason)
	require.Empty(t, withoutHeader.Choices[0].Message.ToolCalls)

	withHeader := call(t, "chat-a")
	require.Equal(t, openAIToolCallFinishReason, withHeader.Choices[0].FinishReason)
	require.Len(t, withHeader.Choices[0].Message.ToolCalls, 1)
}

func TestSendOpenAIStreamIncludesToolCalls(t *testing.T) {
	t.Parallel()

	srv := &Server{minStreamDuration: time.Second, maxStreamDuration: time.Second}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Unix(7, 0).Unix(),
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
						Name:      executeToolName,
						Arguments: `{"command":"echo scaletest"}`,
					},
				}},
			},
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

func TestSendOpenAIStreamPacesTextChunks(t *testing.T) {
	t.Parallel()

	srv := &Server{minStreamDuration: 2 * time.Millisecond, maxStreamDuration: 2 * time.Millisecond}
	writer := httptest.NewRecorder()
	resp := openAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Unix(8, 0).Unix(),
		Model:   "scaletest-model",
		Choices: []openAIResponseChoice{{
			Index:        0,
			Message:      openAIMessage{Role: "assistant", Content: "one two"},
			FinishReason: openAIStopFinishReason,
		}},
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	startedAt := time.Now()
	srv.sendOpenAIStream(ctx, writer, resp)
	require.GreaterOrEqual(t, time.Since(startedAt), time.Millisecond)
	body := writer.Body.String()
	require.Contains(t, body, `"content":"one"`)
	require.Contains(t, body, `"content":" two"`)
	require.Contains(t, body, "[DONE]")
}
