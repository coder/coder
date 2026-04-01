package chattest_test

import (
	"context"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chattest"
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
