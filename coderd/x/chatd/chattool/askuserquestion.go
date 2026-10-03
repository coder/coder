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
	askUserQuestionToolDesc = "Ask the user one or more structured clarification questions during plan mode. Use this instead of listing open questions in prose. Each question should have a short label, a detailed question, and 2-4 answer options."
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

// NewAskUserQuestionTool creates the ask_user_question tool.
func NewAskUserQuestionTool() fantasy.AgentTool {
	return &askUserQuestionTool{}
}

type askUserQuestionTool struct {
	providerOptions fantasy.ProviderOptions
}

func (*askUserQuestionTool) GetType() fantasy.ToolType {
	return fantasy.ToolTypeFunction
}

func (*askUserQuestionTool) GetName() string {
	return askUserQuestionToolName
}

func (*askUserQuestionTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        askUserQuestionToolName,
		Description: askUserQuestionToolDesc,
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

func (*askUserQuestionTool) Run(_ context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var args askUserQuestionArgs
	if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("invalid parameters: %s", err)), nil
	}

	if err := validateAskUserQuestionArgs(args); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	data, err := json.Marshal(map[string]any{"questions": args.Questions})
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal questions: " + err.Error()), nil
	}
	return fantasy.NewTextResponse(string(data)), nil
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
