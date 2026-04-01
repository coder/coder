package chattest_test

import (
	"context"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chattest"
)

func TestAnthropic_Streaming(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicStreamingResponse(
			chattest.AnthropicTextChunks("Hello", " world", "!")...,
		)
	})

	// Create fantasy client pointing to our test server
	client, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "claude-3-opus-20240229")
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

	expectedDeltas := []string{"Hello", " world", "!"}
	deltaIndex := 0

	var allParts []fantasy.StreamPart
	for part := range stream {
		allParts = append(allParts, part)
		if part.Type == fantasy.StreamPartTypeTextDelta {
			require.Less(t, deltaIndex, len(expectedDeltas), "Received more deltas than expected")
			require.Equal(t, expectedDeltas[deltaIndex], part.Delta,
				"Delta at index %d should be %q, got %q", deltaIndex, expectedDeltas[deltaIndex], part.Delta)
			deltaIndex++
		}
	}

	require.Equal(t, len(expectedDeltas), deltaIndex, "Expected %d deltas, got %d. Total parts received: %d", len(expectedDeltas), deltaIndex, len(allParts))
}

func TestAnthropic_ToolCalls(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		switch requestCount.Add(1) {
		case 1:
			return chattest.AnthropicStreamingResponse(
				chattest.AnthropicToolCallChunks("get_weather", `{"location":"San Francisco"}`)...,
			)
		default:
			return chattest.AnthropicStreamingResponse(
				chattest.AnthropicTextChunks("The weather in San Francisco is 72F.")...,
			)
		}
	})

	client, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "claude-3-opus-20240229")
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

	result, err := agent.Stream(context.Background(), fantasy.AgentStreamCall{
		Prompt: "What's the weather in San Francisco?",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, int32(1), toolCallCount.Load(), "expected exactly one tool execution")
	require.GreaterOrEqual(t, requestCount.Load(), int32(2), "expected follow-up model call after tool execution")
}

func TestAnthropic_NonStreaming(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicNonStreamingResponse("Response text")
	})

	// Create fantasy client pointing to our test server
	client, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "claude-3-opus-20240229")
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

func TestAnthropic_Streaming_MismatchReturnsErrorPart(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicNonStreamingResponse("wrong response type")
	})

	client, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "claude-3-opus-20240229")
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
	require.Contains(t, streamErr.Error(), "500 Internal Server Error")
}

func TestAnthropic_NonStreaming_MismatchReturnsError(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
		return chattest.AnthropicStreamingResponse(
			chattest.AnthropicTextChunks("wrong", " response")...,
		)
	})

	client, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "claude-3-opus-20240229")
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
	require.Contains(t, err.Error(), "500 Internal Server Error")
}
