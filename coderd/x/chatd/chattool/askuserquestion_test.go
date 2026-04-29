package chattool //nolint:testpackage // Uses internal symbols.

import (
	"testing"

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
					{Label: " ", Description: "Build the API first."},
					{Label: "Frontend", Description: "Build the UI first."},
				},
			}}},
			wantErr: "questions[0].options[0].label is required",
		},
		{
			name: "OptionDescriptionRequired",
			args: askUserQuestionArgs{Questions: []askUserQuestion{{
				Header:   "Scope",
				Question: "What should we build?",
				Options: []askUserQuestionOption{
					{Label: "Backend", Description: "\t"},
					{Label: "Frontend", Description: "Build the UI first."},
				},
			}}},
			wantErr: "questions[0].options[0].description is required",
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
		{Label: "Backend", Description: "Build the API first."},
		{Label: "Frontend", Description: "Build the UI first."},
		{Label: "Docs", Description: "Write the docs first."},
		{Label: "Tests", Description: "Start with tests first."},
		{Label: "Research", Description: "Investigate the problem first."},
	}

	return append([]askUserQuestionOption(nil), options[:count]...)
}
