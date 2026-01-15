package bridge

import (
	"encoding/json"
	"strings"
)

// ProviderStrategy handles provider-specific message formatting for LLM APIs.
type ProviderStrategy interface {
	DefaultModel() string
	formatMessages(messages []message) []any
	buildRequestBody(model string, messages []any, stream bool) map[string]any
}

type message struct {
	Role    string
	Content string
}

func NewProviderStrategy(provider string) ProviderStrategy {
	switch provider {
	case "anthropic":
		return &anthropicProvider{}
	default:
		return &openAIProvider{}
	}
}

type openAIProvider struct{}

func (*openAIProvider) DefaultModel() string {
	return "gpt-4"
}

func (*openAIProvider) formatMessages(messages []message) []any {
	formatted := make([]any, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	return formatted
}

func (*openAIProvider) buildRequestBody(model string, messages []any, stream bool) map[string]any {
	return map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
}

type anthropicProvider struct{}

func (*anthropicProvider) DefaultModel() string {
	return "claude-3-opus-20240229"
}

func (*anthropicProvider) formatMessages(messages []message) []any {
	formatted := make([]any, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, map[string]any{
			"role": msg.Role,
			"content": []map[string]string{
				{
					"type": "text",
					"text": msg.Content,
				},
			},
		})
	}
	return formatted
}

func (*anthropicProvider) buildRequestBody(model string, messages []any, stream bool) map[string]any {
	return map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": 1024,
		"stream":     stream,
	}
}

// generateConversation creates a conversation with alternating user/assistant
// messages. The content is filled with repeated 'x' characters to reach
// approximately the target size. The last message is always from "user" as
// required by LLM APIs.
func generateConversation(provider ProviderStrategy, targetSize int, numMessages int) []any {
	if targetSize <= 0 {
		return nil
	}
	if numMessages < 1 {
		numMessages = 1
	}

	roles := []string{"user", "assistant"}
	messages := make([]message, numMessages)
	for i := range messages {
		messages[i].Role = roles[i%2]
	}
	// Ensure last message is from user (required for LLM APIs).
	if messages[len(messages)-1].Role != "user" {
		messages[len(messages)-1].Role = "user"
	}

	overhead := measureJSONSize(provider.formatMessages(messages))

	bytesPerMessage := targetSize - overhead
	if bytesPerMessage < 0 {
		bytesPerMessage = 0
	}

	perMessage := bytesPerMessage / len(messages)
	remainder := bytesPerMessage % len(messages)

	for i := range messages {
		size := perMessage
		if i == len(messages)-1 {
			size += remainder
		}
		messages[i].Content = strings.Repeat("x", size)
	}

	return provider.formatMessages(messages)
}

func measureJSONSize(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}
