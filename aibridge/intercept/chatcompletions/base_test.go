package chatcompletions //nolint:testpackage // tests unexported internals

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/coder/aibridge/utils"
)

func TestScanForCorrelatingToolCallID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []openai.ChatCompletionMessageParamUnion
		expected *string
	}{
		{
			name:     "no messages",
			messages: nil,
			expected: nil,
		},
		{
			name: "no tool messages",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.AssistantMessage("hi there"),
			},
			expected: nil,
		},
		{
			name: "single tool message",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("result", "call_abc"),
			},
			expected: utils.PtrTo("call_abc"),
		},
		{
			name: "multiple tool messages returns last",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("first result", "call_first"),
				openai.AssistantMessage("thinking"),
				openai.ToolMessage("second result", "call_second"),
			},
			expected: utils.PtrTo("call_second"),
		},
		{
			name: "last message is not a tool message",
			messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("hello"),
				openai.ToolMessage("first result", "call_first"),
				openai.AssistantMessage("thinking"),
			},
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := &interceptionBase{
				req: &ChatCompletionNewParamsWrapper{
					ChatCompletionNewParams: openai.ChatCompletionNewParams{
						Messages: tc.messages,
					},
				},
			}

			require.Equal(t, tc.expected, base.CorrelatingToolCallID())
		})
	}
}
