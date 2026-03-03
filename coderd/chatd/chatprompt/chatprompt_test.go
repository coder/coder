package chatprompt_test

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/database"
)

func TestConvertMessages_NormalizesAssistantToolCallInput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "{}",
		},
		{
			name:     "invalid json",
			input:    "{\"command\":",
			expected: "{}",
		},
		{
			name:     "non-object json",
			input:    "[]",
			expected: "{}",
		},
		{
			name:     "valid object json",
			input:    "{\"command\":\"ls\"}",
			expected: "{\"command\":\"ls\"}",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assistantContent, err := chatprompt.MarshalContent([]fantasy.Content{
				fantasy.ToolCallContent{
					ToolCallID: "toolu_01C4PqN6F2493pi7Ebag8Vg7",
					ToolName:   "execute",
					Input:      tc.input,
				},
			})
			require.NoError(t, err)

			toolContent, err := chatprompt.MarshalToolResult(
				"toolu_01C4PqN6F2493pi7Ebag8Vg7",
				"execute",
				json.RawMessage(`{"error":"tool call was interrupted before it produced a result"}`),
				true,
			)
			require.NoError(t, err)

			prompt, err := chatprompt.ConvertMessages([]database.ChatMessage{
				{
					Role:       string(fantasy.MessageRoleAssistant),
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    assistantContent,
				},
				{
					Role:       string(fantasy.MessageRoleTool),
					Visibility: database.ChatMessageVisibilityBoth,
					Content:    toolContent,
				},
			})
			require.NoError(t, err)
			require.Len(t, prompt, 2)

			require.Equal(t, fantasy.MessageRoleAssistant, prompt[0].Role)
			toolCalls := chatprompt.ExtractToolCalls(prompt[0].Content)
			require.Len(t, toolCalls, 1)
			require.Equal(t, tc.expected, toolCalls[0].Input)
			require.Equal(t, "execute", toolCalls[0].ToolName)
			require.Equal(t, "toolu_01C4PqN6F2493pi7Ebag8Vg7", toolCalls[0].ToolCallID)

			require.Equal(t, fantasy.MessageRoleTool, prompt[1].Role)
		})
	}
}
