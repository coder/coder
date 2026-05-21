package chatsanitize_test

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
)

func TestSanitizeUnsupportedProviderToolHistory(t *testing.T) {
	t.Parallel()

	providerCall := fantasy.ToolCallPart{
		ToolCallID:       "srvtoolu_ws",
		ToolName:         "web_search",
		Input:            `{"query":"coder"}`,
		ProviderExecuted: true,
	}
	providerResult := fantasy.ToolResultPart{
		ToolCallID:       "srvtoolu_ws",
		Output:           fantasy.ToolResultOutputContentText{Text: `{"ok":true}`},
		ProviderExecuted: true,
		ProviderOptions:  validWebSearchProviderOptionsForTest(),
	}
	resultText := fantasy.TextPart{Text: `{"ok":true}`}
	textPart := fantasy.TextPart{Text: "Here is a summary."}
	userMessage := fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: "search"}},
	}

	testCases := []struct {
		name               string
		provider           string
		messages           []fantasy.Message
		want               []fantasy.Message
		wantRemovedCalls   int
		wantRemovedResults int
		wantDropped        int
	}{
		{
			name:     "anthropic is left alone",
			provider: fantasyanthropic.Name,
			messages: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall, providerResult, textPart},
				},
			},
			want: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall, providerResult, textPart},
				},
			},
		},
		{
			name:     "bedrock strips call and textifies result",
			provider: fantasybedrock.Name,
			messages: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall, providerResult, textPart},
				},
			},
			want: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{resultText, textPart},
				},
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
		},
		{
			name:     "bedrock drops assistant message containing only provider tool blocks",
			provider: fantasybedrock.Name,
			messages: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall},
				},
				userMessage,
			},
			want: []fantasy.Message{
				userMessage,
				userMessage,
			},
			wantRemovedCalls: 1,
			wantDropped:      1,
		},
		{
			name:     "bedrock leaves messages with no provider blocks untouched",
			provider: fantasybedrock.Name,
			messages: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{textPart},
				},
			},
			want: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{textPart},
				},
			},
		},
		{
			name:     "empty provider is a no-op",
			provider: "",
			messages: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall, providerResult},
				},
			},
			want: []fantasy.Message{
				userMessage,
				{
					Role:    fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{providerCall, providerResult},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, stats := chatsanitize.SanitizeUnsupportedProviderToolHistory(tc.provider, tc.messages)
			require.Equal(t, tc.want, got)
			require.Equal(t, chatsanitize.AnthropicProviderToolSanitizationStats{
				RemovedToolCalls:   tc.wantRemovedCalls,
				RemovedToolResults: tc.wantRemovedResults,
				DroppedMessages:    tc.wantDropped,
			}, stats)
		})
	}
}
