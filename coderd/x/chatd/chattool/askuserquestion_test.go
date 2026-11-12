package chattool_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestAskUserQuestion(t *testing.T) {
	t.Parallel()

	const input = `{
		"questions": [{
			"header": "Scope",
			"question": "What should we build?",
			"options": [
				{"label": "Backend", "description": "Build the API first."},
				{"label": "Frontend", "description": "Build the UI first."}
			]
		}]
	}`

	run := func(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
		t.Helper()

		response, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "ask_user_question",
			Input: input,
		})
		require.NoError(t, err)
		return response
	}

	t.Run("Blocking", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionTool()

		response := run(t, tool, input)

		require.False(t, response.IsError)
		require.JSONEq(t, input, response.Content)
	})

	t.Run("InvalidArgs", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionTool()

		response := run(t, tool, `{"questions":[]}`)

		require.True(t, response.IsError)
		require.Equal(t, "questions is required", response.Content)
	})
}
