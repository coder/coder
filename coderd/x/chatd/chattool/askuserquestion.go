package chattool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"
)

const (
	askUserQuestionToolName = "ask_user_question"
	askUserQuestionToolDesc = "Ask the user one or more structured clarification questions during plan mode. Use this instead of listing open questions in prose. Each question needs a short header, the question itself, and 2-4 options. Write each option as one short, complete answer the user could send as their reply."

	askUserQuestionAsyncToolDesc = "Ask the user one or more structured questions without stopping your work. " +
		"The call returns immediately and does not wait for an answer. " +
		"Use it only for decisions that depend on the user's context or preference; if one option is clearly best, proceed with it and state the assumption. " +
		"Continue with work that does not depend on the answer and state the assumptions you make. " +
		"The answer arrives later as a user message that names this tool call's ID, possibly only after your turn ends. " +
		"If you cannot continue without the answer, finish what you can and end your turn with a short note of what is blocked. " +
		"Asking again replaces an earlier unanswered question. " +
		"Each question needs a short header, the question itself, and 2-4 options. Write each option as one short, complete answer the user could send as their reply."
	// maxAsyncQuestionResultBytes matches the smallest tool-result budget in
	// chatloop.ToolResultByteBudget. A longer result would be truncated
	// into invalid JSON, losing the "async" marker the UI and the question
	// push read.
	maxAsyncQuestionResultBytes = 16 << 10
)

var (
	_ fantasy.AgentTool = (*askUserQuestionTool)(nil)
	_ fantasy.Tool      = (*askUserQuestionTool)(nil)
)

type askUserQuestionOption struct {
	Label string `json:"label"`
}

type askUserQuestion struct {
	Header   string                  `json:"header"`
	Question string                  `json:"question"`
	Options  []askUserQuestionOption `json:"options"`
}

type askUserQuestionArgs struct {
	Questions []askUserQuestion `json:"questions"`
}

// AskUserQuestionOptions configures the ask_user_question tool.
type AskUserQuestionOptions struct {
	// Async makes the call return at once without ending the turn. The
	// user answers later in an ordinary user message that names the call.
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
										"description": "The answer exactly as the user would send it: one short, complete sentence, for example \"Build the API first, then the UI\". Keep it under about 80 characters. Do not add a separate explanation.",
									},
								},
								"required": []string{"label"},
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

	result := map[string]any{"questions": args.Questions}
	if t.opts.Async {
		// The UI reads "async" to keep the question answerable while the
		// agent keeps working.
		result["async"] = true
		result["status"] = fmt.Sprintf(
			"The questions are shown to the user. Do not wait for an answer; continue with work that does not depend on it. "+
				"The answer arrives later as a user message that starts with \"Answer to ask_user_question call %s:\".",
			call.ID,
		)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal questions: " + err.Error()), nil
	}
	if t.opts.Async && len(data) > maxAsyncQuestionResultBytes {
		return fantasy.NewTextErrorResponse(fmt.Sprintf(
			"questions are too long: %d bytes, limit %d; shorten the question and option text",
			len(data), maxAsyncQuestionResultBytes,
		)), nil
	}
	return fantasy.NewTextResponse(string(data)), nil
}

// AsyncQuestion returns the first question that an async ask_user_question
// result asks, or "" when result is not one.
func AsyncQuestion(result fantasy.ToolResultContent) (string, error) {
	if result.ToolName != askUserQuestionToolName {
		return "", nil
	}
	output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Result)
	if !ok {
		return "", nil
	}
	var decoded struct {
		Async     bool              `json:"async"`
		Questions []askUserQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(output.Text), &decoded); err != nil {
		return "", xerrors.Errorf("decode ask_user_question result: %w", err)
	}
	if !decoded.Async || len(decoded.Questions) == 0 {
		return "", nil
	}
	return decoded.Questions[0].Question, nil
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
		}
	}
	return nil
}
