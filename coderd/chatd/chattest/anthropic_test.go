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

func newAnthropicLanguageModel(
	t *testing.T,
	mode providerRunMode,
	mockServerURL string,
) fantasy.LanguageModel {
	t.Helper()

	apiKey := "test-key"
	modelName := "claude-3-opus-20240229"
	opts := []fantasyanthropic.Option{}

	if mode.Live {
		apiKey = envOrDefault("ANTHROPIC_API_KEY", "")
		modelName = envOrDefault("ANTHROPIC_TEST_MODEL", "claude-3-5-haiku-latest")
		if baseURL := envOrDefault("ANTHROPIC_BASE_URL", ""); baseURL != "" {
			opts = append(opts, fantasyanthropic.WithBaseURL(baseURL))
		}
	} else {
		opts = append(opts, fantasyanthropic.WithBaseURL(mockServerURL))
	}

	opts = append(opts, fantasyanthropic.WithAPIKey(apiKey))
	client, err := fantasyanthropic.New(opts...)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), modelName)
	require.NoError(t, err)
	return model
}

func TestAnthropic_Streaming(t *testing.T) {
	t.Parallel()

	for _, mode := range providerModes(t, "ANTHROPIC_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
					return chattest.AnthropicStreamingResponse(
						chattest.AnthropicTextChunks("Hello", " world", "!")...,
					)
				})
			}

			model := newAnthropicLanguageModel(t, mode, serverURL)
			stream, err := model.Stream(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Say hello"},
						},
					},
				},
			})
			require.NoError(t, err)

			expectedDeltas := []string{"Hello", " world", "!"}
			deltaIndex := 0
			sawTextDelta := false
			var streamErr error

			for part := range stream {
				if part.Type == fantasy.StreamPartTypeError {
					streamErr = part.Error
					continue
				}
				if part.Type != fantasy.StreamPartTypeTextDelta {
					continue
				}
				sawTextDelta = true
				if mode.Live {
					continue
				}
				require.Less(t, deltaIndex, len(expectedDeltas), "Received more deltas than expected")
				require.Equal(t, expectedDeltas[deltaIndex], part.Delta,
					"Delta at index %d should be %q, got %q", deltaIndex, expectedDeltas[deltaIndex], part.Delta)
				deltaIndex++
			}

			require.NoError(t, streamErr)
			require.True(t, sawTextDelta)
			if !mode.Live {
				require.Equal(t, len(expectedDeltas), deltaIndex, "Expected %d deltas, got %d", len(expectedDeltas), deltaIndex)
			}
		})
	}
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

	for _, mode := range providerModes(t, "ANTHROPIC_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewAnthropic(t, func(req *chattest.AnthropicRequest) chattest.AnthropicResponse {
					return chattest.AnthropicNonStreamingResponse("Response text")
				})
			}

			model := newAnthropicLanguageModel(t, mode, serverURL)
			response, err := model.Generate(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Test message"},
						},
					},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, response)
		})
	}
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
