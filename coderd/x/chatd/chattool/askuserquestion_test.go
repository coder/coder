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
	"golang.org/x/xerrors"

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

		tool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{})

		response := run(t, tool, input)

		require.False(t, response.IsError)
		require.JSONEq(t, input, response.Content)
	})

	t.Run("Async", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: true})

		response := run(t, tool, input)

		require.False(t, response.IsError)

		var result map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
		require.Len(t, result, 3)
		require.Contains(t, result, "questions")
		require.Contains(t, result, "question_id")
		require.Contains(t, result, "status")

		var args map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(input), &args))
		require.JSONEq(t, string(args["questions"]), string(result["questions"]))

		var questionID string
		require.NoError(t, json.Unmarshal(result["question_id"], &questionID))
		_, err := uuid.Parse(questionID)
		require.NoError(t, err)

		var status string
		require.NoError(t, json.Unmarshal(result["status"], &status))
		require.Contains(t, status, fmt.Sprintf("Answer to ask_user_question %s:", questionID))
	})

	t.Run("AsyncMintsNewID", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: true})

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

	t.Run("AsyncTooLong", func(t *testing.T) {
		t.Parallel()

		longQuestion := strings.Repeat("q", 16<<10)
		longInput := strings.Replace(input, "What should we build?", longQuestion, 1)
		asyncTool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: true})
		blockingTool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{})

		asyncResponse := run(t, asyncTool, longInput)
		blockingResponse := run(t, blockingTool, longInput)

		require.True(t, asyncResponse.IsError)
		require.True(t, strings.HasPrefix(asyncResponse.Content, "questions are too long"), asyncResponse.Content)

		require.False(t, blockingResponse.IsError)
		require.JSONEq(t, longInput, blockingResponse.Content)
	})

	t.Run("InvalidArgs", func(t *testing.T) {
		t.Parallel()

		for _, testCase := range []struct {
			name  string
			async bool
		}{
			{name: "Blocking"},
			{name: "Async", async: true},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				tool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: testCase.async})

				response := run(t, tool, `{"questions":[]}`)

				require.True(t, response.IsError)
				require.Equal(t, "questions is required", response.Content)
			})
		}
	})
}

func TestAskUserQuestionInfo(t *testing.T) {
	t.Parallel()

	blockingTool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{})
	asyncTool := chattool.NewAskUserQuestionTool(chattool.AskUserQuestionOptions{Async: true})
	blockingInfo := blockingTool.Info()
	asyncInfo := asyncTool.Info()

	require.NotEqual(t, blockingInfo.Description, asyncInfo.Description)
	require.Equal(t, blockingInfo.Parameters, asyncInfo.Parameters)
	require.Equal(t, blockingInfo.Required, asyncInfo.Required)
}

func TestAsyncQuestion(t *testing.T) {
	t.Parallel()

	const asyncResult = `{
		"question_id": "question-1",
		"questions": [
			{"question": "Which database?"},
			{"question": "Which region?"}
		]
	}`

	tests := []struct {
		name    string
		result  fantasy.ToolResultContent
		want    string
		wantErr bool
	}{
		{
			name: "OtherTool",
			result: fantasy.ToolResultContent{
				ToolName: "read_file",
				Result:   fantasy.ToolResultOutputContentText{Text: "not JSON"},
			},
		},
		{
			name: "NonTextOutput",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentError{Error: xerrors.New("questions is required")},
			},
		},
		{
			name: "InvalidJSON",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentText{Text: "not JSON"},
			},
			wantErr: true,
		},
		{
			name: "BlockingQuestion",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentText{Text: `{"questions":[{"question":"Which database?"}]}`},
			},
		},
		{
			name: "NoQuestions",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentText{Text: `{"question_id":"question-1","questions":[]}`},
			},
		},
		{
			name: "AsyncQuestion",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   fantasy.ToolResultOutputContentText{Text: asyncResult},
			},
			want: "Which database?",
		},
		{
			name: "PointerOutput",
			result: fantasy.ToolResultContent{
				ToolName: "ask_user_question",
				Result:   &fantasy.ToolResultOutputContentText{Text: asyncResult},
			},
			want: "Which database?",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			question, err := chattool.AsyncQuestion(testCase.result)
			if testCase.wantErr {
				require.ErrorContains(t, err, "decode ask_user_question result: ")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, testCase.want, question)
		})
	}
}
