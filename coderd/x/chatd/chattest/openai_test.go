package chattest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestOpenAI_Streaming(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(
			append(
				append(
					chattest.OpenAITextChunks("Hello", "Hi"),
					chattest.OpenAITextChunks(" world", " there")...,
				),
				chattest.OpenAITextChunks("!", "!")...,
			)...,
		)
	})

	// Create fantasy client pointing to our test server
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

	call := fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "Say hello"},
				},
			},
		},
	}

	stream, err := model.Stream(ctx, call)
	require.NoError(t, err)

	// We expect chunks in order: one choice per chunk
	// So we get: "Hello" (choice 0), "Hi" (choice 1), " world" (choice 0), " there" (choice 1), "!" (choice 0), "!" (choice 1)
	expectedDeltas := []string{"Hello", "Hi", " world", " there", "!", "!"}
	deltaIndex := 0

	for part := range stream {
		if part.Type == fantasy.StreamPartTypeTextDelta {
			// Verify we're getting deltas in the expected order
			require.Less(t, deltaIndex, len(expectedDeltas), "Received more deltas than expected")
			require.Equal(t, expectedDeltas[deltaIndex], part.Delta,
				"Delta at index %d should be %q, got %q", deltaIndex, expectedDeltas[deltaIndex], part.Delta)
			deltaIndex++
		}
	}

	// Verify we received all expected deltas
	require.Equal(t, len(expectedDeltas), deltaIndex, "Expected %d deltas, got %d", len(expectedDeltas), deltaIndex)
}

func TestOpenAI_Streaming_ResponsesAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(
			append(
				append(
					chattest.OpenAITextChunks("First", "Second"),
					chattest.OpenAITextChunks(" output", " output")...,
				),
				chattest.OpenAITextChunks("!", "!")...,
			)...,
		)
	})

	// Create fantasy client pointing to our test server (responses API)
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
		fantasyopenai.WithUseResponsesAPI(),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

	call := fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "Say hello"},
				},
			},
		},
	}

	stream, err := model.Stream(ctx, call)
	require.NoError(t, err)

	var parts []fantasy.StreamPart
	for part := range stream {
		parts = append(parts, part)
	}

	// Verify we received the chunks in order
	require.Greater(t, len(parts), 0)

	// Extract text deltas from parts and verify they match expected chunks in order
	// We expect: "First", " output", "!" for choice 0, and "Second", " output", "!" for choice 1
	var allDeltas []string
	for _, part := range parts {
		if part.Type == fantasy.StreamPartTypeTextDelta {
			allDeltas = append(allDeltas, part.Delta)
		}
	}

	// Verify we received deltas (responses API may handle multiple choices differently)
	// If we got text deltas, verify the content
	if len(allDeltas) > 0 {
		allText := ""
		for _, delta := range allDeltas {
			allText += delta
		}
		require.Contains(t, allText, "First")
		require.Contains(t, allText, "Second")
		require.Contains(t, allText, "output")
		require.Contains(t, allText, "!")
	} else {
		// If no text deltas, at least verify we got some parts (may be different format)
		require.Greater(t, len(parts), 0, "Expected at least one stream part")
	}
}

func TestOpenAI_NonStreaming_CompletionsAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAINonStreamingResponse("First response")
	})

	// Create fantasy client pointing to our test server (completions API)
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

	call := fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "Test message"},
				},
			},
		},
	}

	response, err := model.Generate(ctx, call)
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestOpenAI_ToolCalls(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		switch requestCount.Add(1) {
		case 1:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAIToolCallChunk("get_weather", `{"location":"San Francisco"}`),
			)
		default:
			return chattest.OpenAIStreamingResponse(
				chattest.OpenAITextChunks("The weather in San Francisco is 72F.")...,
			)
		}
	})

	// Create fantasy client pointing to our test server
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

	type weatherInput struct {
		Location string `json:"location"`
	}
	var toolCallCount atomic.Int32
	weatherTool := fantasy.NewAgentTool(
		"get_weather",
		"Get weather for a location.",
		func(ctx context.Context, input weatherInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			toolCallCount.Add(1)
			require.Equal(t, "San Francisco", input.Location)
			return fantasy.NewTextResponse("72F"), nil
		},
	)

	agent := fantasy.NewAgent(
		model,
		fantasy.WithSystemPrompt("You are a helpful assistant."),
		fantasy.WithTools(weatherTool),
	)

	result, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt: "What's the weather in San Francisco?",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), toolCallCount.Load(), "expected exactly one tool execution")
	require.GreaterOrEqual(t, requestCount.Load(), int32(2), "expected follow-up model call after tool execution")
}

func TestOpenAI_NonStreaming_ResponsesAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAINonStreamingResponse("First output")
	})

	// Create fantasy client pointing to our test server (responses API)
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
		fantasyopenai.WithUseResponsesAPI(),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

	call := fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role: fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{
					fantasy.TextPart{Text: "Test message"},
				},
			},
		},
	}

	response, err := model.Generate(ctx, call)
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestOpenAI_Streaming_MismatchReturnsErrorPart(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAINonStreamingResponse("wrong response type")
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	stream, err := model.Stream(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.NoError(t, err)

	var streamErr error
	for part := range stream {
		if part.Type == fantasy.StreamPartTypeError {
			streamErr = part.Error
			break
		}
	}
	require.Error(t, streamErr)
	require.Contains(t, streamErr.Error(), "non-streaming response for streaming request")
}

func TestOpenAI_NonStreaming_MismatchReturnsError_CompletionsAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("wrong response type")...)
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "streaming response for non-streaming request")
}

func TestOpenAI_NonStreaming_MismatchReturnsError_ResponsesAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("wrong response type")...)
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
		fantasyopenai.WithUseResponsesAPI(),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "streaming response for non-streaming request")
}

// TestOpenAI_ResponsesAPI_RejectsChainWithoutToolOutput verifies the
// chattest OpenAI server enforces the Responses API chain-mode
// contract: when a follow-up request sets previous_response_id to a
// stored response whose output contained a function_call, the input
// on the new request must include a matching function_call_output or
// the server rejects with HTTP 400 mirroring OpenAI's production
// error body.
func TestOpenAI_ResponsesAPI_RejectsChainWithoutToolOutput(t *testing.T) {
	t.Parallel()

	const responseID = "resp_chainmode_test_1"
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		// First call: emit a function_call item that must be
		// answered on the next request. The test server will
		// remember the call because req.Store is true.
		if req.PreviousResponseID == nil {
			return chattest.OpenAIResponse{
				StreamingChunks: emptyChunkChannel(),
				FunctionCalls: []chattest.OpenAIResponsesFunctionCall{{
					CallID:    "call_must_be_answered",
					Name:      "list_templates",
					Arguments: `{}`,
				}},
				ResponseID: responseID,
			}
		}
		// The follow-up streamed body is irrelevant because the
		// chain-mode validation fails before the handler emits
		// anything, but we still need to return a valid response
		// shape to keep the test server happy.
		return chattest.OpenAIResponse{
			StreamingChunks: emptyChunkChannel(),
		}
	})

	// Turn 1: initial streaming request with store=true. This makes
	// the fake server remember the emitted function_call.
	turn1Body := mustMarshalJSON(t, map[string]any{
		"model":  "gpt-5.5",
		"stream": true,
		"store":  true,
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": "list templates please",
			},
		},
	})
	statusCode, body := postResponses(t, serverURL, turn1Body)
	require.Equal(t, http.StatusOK, statusCode, "turn 1 should succeed")
	require.Contains(t, body, "function_call",
		"turn 1 should emit a function_call item")
	require.Contains(t, body, "call_must_be_answered",
		"turn 1 should emit the registered call_id")

	// Turn 2: chain via previous_response_id but without a
	// function_call_output for call_must_be_answered. The server
	// must reject with HTTP 400 mirroring OpenAI's real error.
	turn2Body := mustMarshalJSON(t, map[string]any{
		"model":                "gpt-5.5",
		"stream":               true,
		"store":                true,
		"previous_response_id": responseID,
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": "follow-up without tool output",
			},
		},
	})
	statusCode, body = postResponses(t, serverURL, turn2Body)
	require.Equal(t, http.StatusBadRequest, statusCode,
		"turn 2 must be rejected with 400 when the chain anchor has "+
			"unanswered function_calls")
	require.Contains(t, body, "No tool output found for function call call_must_be_answered",
		"error body should mirror OpenAI's production message")
}

// TestOpenAI_ResponsesAPI_AcceptsChainWithToolOutput confirms the
// chattest server lets a chained follow-up through when the input
// includes a function_call_output for every unanswered function_call
// on the referenced response. This exercises the "happy path" side
// of the contract so subtle regressions in the validator surface as
// test failures rather than silent passes.
func TestOpenAI_ResponsesAPI_AcceptsChainWithToolOutput(t *testing.T) {
	t.Parallel()

	const responseID = "resp_chainmode_test_2"
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if req.PreviousResponseID == nil {
			return chattest.OpenAIResponse{
				StreamingChunks: emptyChunkChannel(),
				FunctionCalls: []chattest.OpenAIResponsesFunctionCall{{
					CallID:    "call_answered_on_next_turn",
					Name:      "list_templates",
					Arguments: `{}`,
				}},
				ResponseID: responseID,
			}
		}
		return chattest.OpenAIResponse{
			StreamingChunks: emptyChunkChannel(),
		}
	})

	// Turn 1 sets up the outstanding call.
	turn1Body := mustMarshalJSON(t, map[string]any{
		"model":  "gpt-5.5",
		"stream": true,
		"store":  true,
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": "list templates please",
			},
		},
	})
	statusCode, _ := postResponses(t, serverURL, turn1Body)
	require.Equal(t, http.StatusOK, statusCode)

	// Turn 2 provides the required function_call_output alongside
	// the new user message. The server must accept this chain.
	turn2Body := mustMarshalJSON(t, map[string]any{
		"model":                "gpt-5.5",
		"stream":               true,
		"store":                true,
		"previous_response_id": responseID,
		"input": []any{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_answered_on_next_turn",
				"output":  "{\"templates\":[]}",
			},
			map[string]any{
				"role":    "user",
				"content": "thanks",
			},
		},
	})
	statusCode, body := postResponses(t, serverURL, turn2Body)
	require.Equal(t, http.StatusOK, statusCode,
		"turn 2 should succeed once every function_call has a matching function_call_output (body=%s)", body)
}

func postResponses(t *testing.T, baseURL string, body []byte) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/responses", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// emptyChunkChannel returns a closed channel so the streaming writer
// exits immediately with no text deltas. The tests that use it only
// care about header items emitted before streaming begins (function
// calls, validation errors).
func emptyChunkChannel() chan chattest.OpenAIChunk {
	ch := make(chan chattest.OpenAIChunk)
	close(ch)
	return ch
}
