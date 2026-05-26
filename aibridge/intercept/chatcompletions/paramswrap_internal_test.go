package chatcompletions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
)

func TestOpenAILastUserPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wrapper     *ChatCompletionNewParamsWrapper
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil struct",
			expectError: true,
			errorMsg:    "nil struct",
		},
		{
			name: "no messages",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{},
				},
			},
			expectError: true,
			errorMsg:    "no messages",
		},
		{
			name: "last message not from user",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("user message"),
						openai.AssistantMessage("assistant message"),
					},
				},
			},
		},
		{
			name: "user message with string content",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage("Hello, world!"),
					},
				},
			},
			expected: "Hello, world!",
		},
		{
			name: "user message with empty string",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage(""),
					},
				},
			},
		},
		{
			name: "user message with array content - text at end",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
							openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
								URL: "https://example.com/image.png",
							}),
							openai.TextContentPart("First text"),
							openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
								URL: "https://example.com/image2.png",
							}),
							openai.TextContentPart("Last text"),
						}),
					},
				},
			},
			expected: "Last text",
		},
		{
			name: "user message with array content - no text",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
							openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
								URL: "https://example.com/image.png",
							}),
						}),
					},
				},
			},
		},
		{
			name: "user message with empty array",
			wrapper: &ChatCompletionNewParamsWrapper{
				ChatCompletionNewParams: openai.ChatCompletionNewParams{
					Messages: []openai.ChatCompletionMessageParamUnion{
						openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{}),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := tt.wrapper.lastUserPrompt()

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				if tt.expected == "" {
					require.Nil(t, result)
				} else {
					require.NotNil(t, result)
					require.Equal(t, tt.expected, *result)
				}
			}
		})
	}
}

// generatePayload creates a JSON payload with the specified number of messages.
// Messages alternate between user and assistant roles to simulate a conversation.
func generatePayload(messageCount int) []byte {
	var messages []string
	for i := range messageCount {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		// Use realistic message content size
		content := fmt.Sprintf("This is message number %d with some realistic content that might appear in a conversation.", i+1)
		messages = append(messages, fmt.Sprintf(`{"role": %q, "content": %q}`, role, content))
	}

	return []byte(fmt.Sprintf(`{
		"model": "gpt-4",
		"stream": true,
		"messages": [%s]
	}`, strings.Join(messages, ",")))
}

func BenchmarkChatCompletionNewParamsWrapper_UnmarshalJSON(b *testing.B) {
	messageCounts := []int{1, 10, 20, 50}

	for _, count := range messageCounts {
		payload := generatePayload(count)

		b.Run(fmt.Sprintf("messages=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var wrapper ChatCompletionNewParamsWrapper
				_ = wrapper.UnmarshalJSON(payload)
			}
		})
	}
}
