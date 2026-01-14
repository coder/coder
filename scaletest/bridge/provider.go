package bridge

import (
	"encoding/json"
	"math/rand"
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

const (
	minLinesPerMessage = 1
	maxLinesPerMessage = 100
	minCharsPerLine    = 40
	maxCharsPerLine    = 120
)

const printableChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .,!?;:'-"

// generateConversation creates interleaving user/assistant messages with random
// content. Each message has 1-100 lines, each line has 40-120 random characters.
// The last message is padded or trimmed so that the JSON-encoded messages array
// matches exactly targetSize bytes.
func generateConversation(rng *rand.Rand, provider ProviderStrategy, targetSize int) ([]any, error) {
	if targetSize <= 0 {
		return nil, nil
	}

	var messages []message
	roles := []string{"user", "assistant"}
	currentRole := 0
	totalContent := 0

	targetContentSize := targetSize * 80 / 100

	for totalContent < targetContentSize {
		content := generateRandomMessageContent(rng)
		messages = append(messages, message{
			Role:    roles[currentRole],
			Content: content,
		})
		totalContent += len(content)
		currentRole = (currentRole + 1) % 2
	}

	if len(messages) == 0 {
		messages = append(messages, message{
			Role:    "user",
			Content: "Hello",
		})
	}

	if messages[len(messages)-1].Role != "user" {
		messages = append(messages, message{
			Role:    "user",
			Content: "Continue",
		})
	}

	formatted := provider.formatMessages(messages)
	currentSize := measureJSONSize(formatted)

	lastMsg := &messages[len(messages)-1]
	if currentSize < targetSize {
		// Need to pad the last message.
		padding := generatePadding(rng, targetSize-currentSize)
		lastMsg.Content += padding
	} else if currentSize > targetSize {
		// Need to trim the last message.
		excess := currentSize - targetSize
		if len(lastMsg.Content) > excess {
			lastMsg.Content = lastMsg.Content[:len(lastMsg.Content)-excess]
		} else {
			// If we can't trim enough, just use minimal content.
			lastMsg.Content = "x"
		}
	}

	// Fine-tune: measure again and adjust character by character if needed.
	formatted = provider.formatMessages(messages)
	currentSize = measureJSONSize(formatted)

	for currentSize < targetSize {
		lastMsg.Content += "x"
		formatted = provider.formatMessages(messages)
		currentSize = measureJSONSize(formatted)
	}

	for currentSize > targetSize && len(lastMsg.Content) > 1 {
		lastMsg.Content = lastMsg.Content[:len(lastMsg.Content)-1]
		formatted = provider.formatMessages(messages)
		currentSize = measureJSONSize(formatted)
	}

	return formatted, nil
}

func generateRandomMessageContent(rng *rand.Rand) string {
	numLines := minLinesPerMessage + rng.Intn(maxLinesPerMessage-minLinesPerMessage+1)
	var sb strings.Builder

	for i := 0; i < numLines; i++ {
		lineLen := minCharsPerLine + rng.Intn(maxCharsPerLine-minCharsPerLine+1)
		for j := 0; j < lineLen; j++ {
			_ = sb.WriteByte(printableChars[rng.Intn(len(printableChars))])
		}
		if i < numLines-1 {
			_ = sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// generatePadding creates random padding text of approximately the given size.
func generatePadding(rng *rand.Rand, size int) string {
	if size <= 0 {
		return ""
	}

	var sb strings.Builder
	sb.Grow(size)

	_ = sb.WriteByte('\n')

	for sb.Len() < size {
		_ = sb.WriteByte(printableChars[rng.Intn(len(printableChars))])
	}

	return sb.String()
}

func measureJSONSize(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}
