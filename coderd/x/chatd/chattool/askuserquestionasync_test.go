package chattool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestAskUserQuestionAsync(t *testing.T) {
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
			Name:  tool.Info().Name,
			Input: input,
		})
		require.NoError(t, err)
		return response
	}

	t.Run("ReturnsQuestionIDAndStatus", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionAsyncTool()

		response := run(t, tool, input)

		require.False(t, response.IsError)

		var result map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Len(t, result, 2)
		require.Contains(t, result, "question_id")
		require.Contains(t, result, "status")

		var questionID string
		require.NoError(t, json.Unmarshal(result["question_id"], &questionID))
		_, err := uuid.Parse(questionID)
		require.NoError(t, err)

		var status string
		require.NoError(t, json.Unmarshal(result["status"], &status))
		require.Contains(t, status, fmt.Sprintf("Answer to ask_user_question_async %s:", questionID))
	})

	t.Run("MintsNewID", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionAsyncTool()

		var questionIDs []string
		for range 2 {
			response := run(t, tool, input)
			require.False(t, response.IsError)

			var result struct {
				QuestionID string `json:"question_id"`
			}
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			require.NotEmpty(t, result.QuestionID)
			questionIDs = append(questionIDs, result.QuestionID)
		}

		require.NotEqual(t, questionIDs[0], questionIDs[1])
	})

	t.Run("AcceptsLongQuestions", func(t *testing.T) {
		t.Parallel()

		longQuestion := strings.Repeat("q", 20_000)
		longInput := strings.Replace(input, "What should we build?", longQuestion, 1)
		tool := chattool.NewAskUserQuestionAsyncTool()

		response := run(t, tool, longInput)

		require.False(t, response.IsError, response.Content)

		var result struct {
			QuestionID string `json:"question_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.NotEmpty(t, result.QuestionID)
	})

	t.Run("InvalidArgs", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionAsyncTool()

		response := run(t, tool, `{"questions":[]}`)

		require.True(t, response.IsError)
		require.Equal(t, "questions is required", response.Content)
	})

	t.Run("Info", func(t *testing.T) {
		t.Parallel()

		blockingTool := chattool.NewAskUserQuestionTool()
		asyncTool := chattool.NewAskUserQuestionAsyncTool()
		blockingInfo := blockingTool.Info()
		asyncInfo := asyncTool.Info()

		require.NotEqual(t, blockingInfo.Name, asyncInfo.Name)
		require.NotEqual(t, blockingInfo.Description, asyncInfo.Description)
		require.Equal(t, blockingInfo.Parameters, asyncInfo.Parameters)
		require.Equal(t, blockingInfo.Required, asyncInfo.Required)
	})
}

func TestFirstAskedQuestion(t *testing.T) {
	t.Parallel()

	const input = `{
		"questions": [
			{
				"header": "Database",
				"question": "Which database?",
				"options": [
					{"label": "Postgres", "description": "Use Postgres."},
					{"label": "SQLite", "description": "Use SQLite."}
				]
			},
			{
				"header": "Region",
				"question": "Which region?",
				"options": [
					{"label": "US", "description": "Use the US."},
					{"label": "EU", "description": "Use the EU."}
				]
			}
		]
	}`

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "TwoQuestions",
			input: input,
			want:  "Which database?",
		},
		{
			name:  "NoQuestions",
			input: `{"questions":[]}`,
		},
		{
			name:    "InvalidJSON",
			input:   "not JSON",
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			question, err := chattool.FirstAskedQuestion(testCase.input)

			if testCase.wantErr {
				require.ErrorContains(t, err, "decode ask_user_question_async input: ")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, testCase.want, question)
		})
	}
}
