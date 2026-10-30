package chattool

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestValidateAskUserQuestionArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    askUserQuestionArgs
		wantErr string
	}{
		{
			name:    "QuestionsRequired",
			args:    askUserQuestionArgs{},
			wantErr: "questions is required",
		},
		{
			name: "HeaderRequired",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   " \t ",
				Question: "What should we build?",
				Options:  validAskUserQuestionOptions(2),
			}}},
			wantErr: "questions[0].header is required",
		},
		{
			name: "QuestionRequired",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "\n\t ",
				Options:  validAskUserQuestionOptions(2),
			}}},
			wantErr: "questions[0].question is required",
		},
		{
			name: "TooFewOptions",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options:  validAskUserQuestionOptions(1),
			}}},
			wantErr: "questions[0].options must contain 2-4 items",
		},
		{
			name: "TooManyOptions",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options:  validAskUserQuestionOptions(5),
			}}},
			wantErr: "questions[0].options must contain 2-4 items",
		},
		{
			name: "OptionLabelRequired",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options: []askUserQuestionOption{
					{Label: " "},
					{Label: "Build the UI first."},
				},
			}}},
			wantErr: "questions[0].options[0].label is required",
		},
		{
			name: "ValidTwoOptions",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options:  validAskUserQuestionOptions(2),
			}}},
		},
		{
			name: "ValidFourOptions",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options:  validAskUserQuestionOptions(4),
			}}},
		},
		{
			name: "SecondQuestionInvalid",
			args: askUserQuestionArgs{Questions: []askUserQuestion{
				{
					Header:   "Scope",
					Question: "What should we build?",
					Options:  validAskUserQuestionOptions(2),
				},
				{
					Header:   "Timeline",
					Question: "\t ",
					Options:  validAskUserQuestionOptions(2),
				},
			}},
			wantErr: "questions[1].question is required",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validateAskUserQuestionArgs(testCase.args)
			if testCase.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.EqualError(t, err, testCase.wantErr)
		})
	}
}

func validAskUserQuestionOptions(count int) []askUserQuestionOption {
	options := []askUserQuestionOption{
		{Label: "Build the API first."},
		{Label: "Build the UI first."},
		{Label: "Write the docs first."},
		{Label: "Start with tests first."},
		{Label: "Investigate the problem first."},
	}

	return append([]askUserQuestionOption(nil), options[:count]...)
}

func TestAskUserQuestionToolRun(t *testing.T) {
	t.Parallel()

	validInput := `{"questions":[{"header":"Scope","question":"What should we build?","options":[` +
		`{"label":"Build the API first."},{"label":"Build the UI first."}]}]}`
	// Models may still send an option description, which must not fail
	// the call.
	legacyInput := `{"questions":[{"header":"Scope","question":"What should we build?","options":[` +
		`{"label":"Backend","description":"Build the API first."},` +
		`{"label":"Frontend","description":"Build the UI first."}]}]}`

	tests := []struct {
		name      string
		async     bool
		input     string
		wantAsked string
		wantError bool
	}{
		{name: "Blocking", input: validInput},
		{name: "Async", async: true, input: validInput, wantAsked: "What should we build?"},
		{name: "LegacyDescription", input: legacyInput},
		{name: "AsyncInvalid", async: true, input: `{"questions":[]}`, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tool := NewAskUserQuestionTool(AskUserQuestionOptions{Async: tt.async})
			response, err := tool.Run(t.Context(), fantasy.ToolCall{
				ID:    "call-123",
				Name:  askUserQuestionToolName,
				Input: tt.input,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantError, response.IsError)
			if tt.wantError {
				return
			}

			asked, err := AsyncQuestion(fantasy.ToolResultContent{
				ToolName: askUserQuestionToolName,
				Result:   fantasy.ToolResultOutputContentText{Text: response.Content},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantAsked, asked)

			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
			require.Contains(t, result, "questions")
			if !tt.async {
				// Plan mode rows have no async marker, so the UI keeps
				// rendering them as blocking questions.
				require.Len(t, result, 1)
				return
			}
			require.Equal(t, true, result["async"])
			require.Contains(t, result["status"], "Answer to ask_user_question call call-123:")
		})
	}
}

func TestAskUserQuestionOptionSchemaRequiresOnlyLabel(t *testing.T) {
	t.Parallel()

	params := NewAskUserQuestionTool(AskUserQuestionOptions{}).Info().Parameters
	questions, ok := params["questions"].(map[string]any)
	require.True(t, ok)
	question, ok := questions["items"].(map[string]any)
	require.True(t, ok)
	questionProperties, ok := question["properties"].(map[string]any)
	require.True(t, ok)
	options, ok := questionProperties["options"].(map[string]any)
	require.True(t, ok)
	option, ok := options["items"].(map[string]any)
	require.True(t, ok)

	require.Equal(t, []string{"label"}, option["required"])
	require.NotContains(t, option["properties"], "description")
}
