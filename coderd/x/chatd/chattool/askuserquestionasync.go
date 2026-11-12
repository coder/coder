package chattool

import (
	"context"
	"encoding/json"
	"fmt"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// AskUserQuestionAsyncName is the name of the ask_user_question_async tool.
const AskUserQuestionAsyncName = "ask_user_question_async"

const (
	askUserQuestionAsyncToolDesc = "Ask the user one or more structured questions without ending your turn. " +
		"The call returns at once and the answer arrives later as a user message. " +
		"Use it only for decisions that depend on the user's context or preference; if one option is clearly best, proceed with it and state the assumption. " +
		"Asking again replaces an earlier unanswered question."
	askUserQuestionAsyncStatusFormat = "The questions are shown to the user. " +
		"Continue with work that does not depend on the answer, and state the assumptions you make. " +
		"If you cannot continue without the answer, finish what you can and end your turn with a note of what is blocked. " +
		"The answer arrives as a user message that starts with \"Answer to ask_user_question_async %s:\"."
)

var (
	_ fantasy.AgentTool = (*askUserQuestionAsyncTool)(nil)
	_ fantasy.Tool      = (*askUserQuestionAsyncTool)(nil)
)

type askUserQuestionAsyncResult struct {
	QuestionID string `json:"question_id"`
	Status     string `json:"status"`
}

// NewAskUserQuestionAsyncTool creates the ask_user_question_async tool.
// The call returns a question_id without ending the turn. The user's
// answer arrives later as a user message.
func NewAskUserQuestionAsyncTool() fantasy.AgentTool {
	return &askUserQuestionAsyncTool{}
}

type askUserQuestionAsyncTool struct {
	providerOptions fantasy.ProviderOptions
}

func (*askUserQuestionAsyncTool) GetType() fantasy.ToolType {
	return fantasy.ToolTypeFunction
}

func (*askUserQuestionAsyncTool) GetName() string {
	return AskUserQuestionAsyncName
}

func (*askUserQuestionAsyncTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        AskUserQuestionAsyncName,
		Description: askUserQuestionAsyncToolDesc,
		Parameters:  askUserQuestionParameters(),
		Required:    []string{"questions"},
	}
}

func (*askUserQuestionAsyncTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var args askUserQuestionArgs
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if err := validateAskUserQuestionArgs(args); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return asyncQuestionResponse(), nil
}

// asyncQuestionResponse returns the result of an async question, with a
// new question_id.
func asyncQuestionResponse() fantasy.ToolResponse {
	questionID := uuid.NewString()
	result := askUserQuestionAsyncResult{
		QuestionID: questionID,
		Status:     fmt.Sprintf(askUserQuestionAsyncStatusFormat, questionID),
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal questions: " + err.Error())
	}

	return fantasy.NewTextResponse(string(data))
}

// FirstAskedQuestion returns the text of the first question in the
// input of an ask_user_question call.
func FirstAskedQuestion(input string) (string, error) {
	var args askUserQuestionArgs
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", xerrors.Errorf("decode ask_user_question input: %w", err)
	}

	if len(args.Questions) == 0 {
		return "", nil
	}

	return args.Questions[0].Question, nil
}

func (t *askUserQuestionAsyncTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *askUserQuestionAsyncTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}
