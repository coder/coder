package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	askUserQuestionToolName      = "ask_user_question"
	askUserQuestionToolDesc      = "Ask the user one or more structured clarification questions during plan mode. Use this instead of listing open questions in prose. Each question should have a short label, a detailed question, and 2-4 answer options."
	askUserQuestionAsyncToolDesc = "Ask the user one or more structured questions without ending your turn. " +
		"The call returns at once and the answer arrives later as a user message. " +
		"Use it only for decisions that depend on the user's context or preference; if one option is clearly best, proceed with it and state the assumption. " +
		"Asking again replaces an earlier unanswered question."
	askUserQuestionAsyncStatusFormat = "The questions are shown to the user. " +
		"Continue with work that does not depend on the answer, and state the assumptions you make. " +
		"If you cannot continue without the answer, finish what you can and end your turn with a note of what is blocked. " +
		"The answer arrives as a user message that starts with \"Answer to ask_user_question %s:\"."

	// maxAsyncQuestionResultBytes is the smallest tool-result budget that
	// chatloop.ToolResultByteBudget returns. A longer result would be
	// truncated into invalid JSON and lose its question_id.
	maxAsyncQuestionResultBytes = 16 << 10
)

var (
	_ fantasy.AgentTool = (*askUserQuestionTool)(nil)
	_ fantasy.Tool      = (*askUserQuestionTool)(nil)
)

type askUserQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type askUserQuestion struct {
	Header   string                  `json:"header"`
	Question string                  `json:"question"`
	Options  []askUserQuestionOption `json:"options"`
}

type askUserQuestionArgs struct {
	Questions []askUserQuestion `json:"questions"`
}

type askUserQuestionAsyncResult struct {
	Questions  []askUserQuestion `json:"questions"`
	QuestionID string            `json:"question_id"`
	Status     string            `json:"status"`
}

// AskUserQuestionOptions configures the ask_user_question tool.
type AskUserQuestionOptions struct {
	// Async selects the description and the result for a question that
	// does not end the turn. The result carries a question_id that the
	// user's answer refers to.
	Async bool
}

// NewAskUserQuestionTool creates the ask_user_question tool.
func NewAskUserQuestionTool(opts AskUserQuestionOptions) fantasy.AgentTool {
	return &askUserQuestionTool{opts: opts}
}

type askUserQuestionTool struct {
	opts            AskUserQuestionOptions
	providerOptions fantasy.ProviderOptions
}

func (*askUserQuestionTool) GetType() fantasy.ToolType {
	return fantasy.ToolTypeFunction
}

func (*askUserQuestionTool) GetName() string {
	return askUserQuestionToolName
}

func (t *askUserQuestionTool) Info() fantasy.ToolInfo {
	description := askUserQuestionToolDesc
	if t.opts.Async {
		description = askUserQuestionAsyncToolDesc
	}

	return fantasy.ToolInfo{
		Name:        askUserQuestionToolName,
		Description: description,
		Parameters: map[string]any{
			"questions": map[string]any{
				"type":        "array",
				"description": "The structured clarification questions to present to the user.",
				"minItems":    1,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"header": map[string]any{
							"type":        "string",
							"description": "A short label for the question.",
						},
						"question": map[string]any{
							"type":        "string",
							"description": "The detailed question text.",
						},
						"options": map[string]any{
							"type":        "array",
							"description": "The answer options the user can choose from. Do not include an 'Other' or freeform option; one is provided automatically by the UI.",
							"minItems":    2,
							"maxItems":    4,
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"label": map[string]any{
										"type":        "string",
										"description": "A short answer label.",
									},
									"description": map[string]any{
										"type":        "string",
										"description": "More detail about what this option means.",
									},
								},
								"required": []string{"label", "description"},
							},
						},
					},
					"required": []string{"header", "question", "options"},
				},
			},
		},
		Required: []string{"questions"},
	}
}

func (t *askUserQuestionTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var args askUserQuestionArgs
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if err := validateAskUserQuestionArgs(args); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	if t.opts.Async {
		return asyncQuestionResponse(args.Questions), nil
	}

	data, err := json.Marshal(map[string]any{"questions": args.Questions})
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal questions: " + err.Error()), nil
	}
	return fantasy.NewTextResponse(string(data)), nil
}

// asyncQuestionResponse returns the result of an async question, with a
// new question_id, or an error response when the result does not fit in
// maxAsyncQuestionResultBytes.
func asyncQuestionResponse(questions []askUserQuestion) fantasy.ToolResponse {
	questionID := uuid.NewString()
	result := askUserQuestionAsyncResult{
		Questions:  questions,
		QuestionID: questionID,
		Status:     fmt.Sprintf(askUserQuestionAsyncStatusFormat, questionID),
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal questions: " + err.Error())
	}

	if len(data) > maxAsyncQuestionResultBytes {
		message := fmt.Sprintf(
			"questions are too long: %d bytes, limit %d; shorten the question and option text",
			len(data), maxAsyncQuestionResultBytes,
		)
		return fantasy.NewTextErrorResponse(message)
	}

	return fantasy.NewTextResponse(string(data))
}

// AsyncQuestion returns the text of the first question in an async
// ask_user_question result. It returns "" when result is not an async
// ask_user_question result.
func AsyncQuestion(result fantasy.ToolResultContent) (string, error) {
	if result.ToolName != askUserQuestionToolName {
		return "", nil
	}

	output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Result)
	if !ok {
		return "", nil
	}

	var questionResult askUserQuestionAsyncResult
	if err := json.Unmarshal([]byte(output.Text), &questionResult); err != nil {
		return "", xerrors.Errorf("decode ask_user_question result: %w", err)
	}

	if questionResult.QuestionID == "" || len(questionResult.Questions) == 0 {
		return "", nil
	}

	return questionResult.Questions[0].Question, nil
}

func (t *askUserQuestionTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *askUserQuestionTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}

func validateAskUserQuestionArgs(args askUserQuestionArgs) error {
	if len(args.Questions) == 0 {
		return xerrors.New("questions is required")
	}
	for i, question := range args.Questions {
		if strings.TrimSpace(question.Header) == "" {
			return xerrors.Errorf("questions[%d].header is required", i)
		}
		if strings.TrimSpace(question.Question) == "" {
			return xerrors.Errorf("questions[%d].question is required", i)
		}
		if len(question.Options) < 2 || len(question.Options) > 4 {
			return xerrors.Errorf("questions[%d].options must contain 2-4 items", i)
		}
		for j, option := range question.Options {
			if strings.TrimSpace(option.Label) == "" {
				return xerrors.Errorf("questions[%d].options[%d].label is required", i, j)
			}
			if strings.TrimSpace(option.Description) == "" {
				return xerrors.Errorf("questions[%d].options[%d].description is required", i, j)
			}
		}
	}
	return nil
}
