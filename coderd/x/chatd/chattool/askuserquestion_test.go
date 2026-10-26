package chattool_test

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// chatloop truncates a result longer than its budget into invalid JSON,
// which hides the "async" marker from the UI and the question push.
func TestAsyncAskUserQuestionResultFitsSmallestBudget(t *testing.T) {
	t.Parallel()

	budget := chatloop.ToolResultByteBudget(1)
	tests := []struct {
		name          string
		questionBytes int
		wantError     bool
	}{
		{name: "Fits", questionBytes: budget / 2},
		{name: "TooLong", questionBytes: budget, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			question := strings.Repeat("q", tt.questionBytes)
			input, err := json.Marshal(map[string]any{"questions": []map[string]any{{
				"header":   "Scope",
				"question": question,
				"options": []map[string]string{
					{"label": "Backend", "description": "Build the API first."},
					{"label": "Frontend", "description": "Build the UI first."},
				},
			}}})
			require.NoError(t, err)

			tool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: true})
			response, err := tool.Run(t.Context(), fantasy.ToolCall{
				ID:    "call-123",
				Name:  "ask_user_question",
				Input: string(input),
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantError, response.IsError)
			if tt.wantError {
				return
			}

			text, truncated := chatloop.TruncateToolResultText(response.Content, budget)
			require.False(t, truncated)
			asked, err := chattool.AsyncQuestion(fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentText{Text: text},
			})
			require.NoError(t, err)
			require.Equal(t, question, asked)
		})
	}
}
