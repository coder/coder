package chatadvisor

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
)

const advisorQuestionMaxRunes = 2000

// ToolOptions configures the built-in advisor tool.
type ToolOptions struct {
	Runtime                 *Runtime
	GetConversationSnapshot func() []fantasy.Message
}

// Tool returns a fantasy.AgentTool that asks a nested model for concise
// strategic guidance. The nested advisor sees recent conversation
// context, runs without tools, and is limited to a single model step.
func Tool(opts ToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"advisor",
		"Ask a stronger model for strategic guidance about planning, architecture, tradeoffs, or debugging strategy. Provide a brief question. The advisor sees recent conversation context, runs without tools for a single step, and responds to the parent agent rather than the end user.",
		func(ctx context.Context, args AdvisorArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if opts.Runtime == nil {
				return fantasy.NewTextErrorResponse("advisor runtime is not configured"), nil
			}
			if opts.GetConversationSnapshot == nil {
				return fantasy.NewTextErrorResponse("conversation snapshot provider is not configured"), nil
			}

			question := strings.TrimSpace(args.Question)
			if question == "" {
				return fantasy.NewTextErrorResponse("question is required"), nil
			}
			if utf8.RuneCountInString(question) > advisorQuestionMaxRunes {
				return fantasy.NewTextErrorResponse("question must be 2000 characters or fewer"), nil
			}

			result, err := opts.Runtime.RunAdvisor(ctx, question, opts.GetConversationSnapshot())
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return jsonToolResponse(advisorResultMap(result)), nil
		},
	)
}

// jsonToolResponse builds a fantasy.ToolResponse from a JSON-serializable
// map. Mirrors chattool.toolResponse but kept here to avoid a cyclic
// dependency on chattool (which depends on chatprompt, which depends on
// chattool again via attachment helpers).
func jsonToolResponse(result map[string]any) fantasy.ToolResponse {
	data, err := json.Marshal(result)
	if err != nil {
		return fantasy.NewTextResponse("{}")
	}
	return fantasy.NewTextResponse(string(data))
}

func advisorResultMap(result AdvisorResult) map[string]any {
	payload := map[string]any{
		"type":           result.Type,
		"remaining_uses": result.RemainingUses,
	}
	if result.Advice != "" {
		payload["advice"] = result.Advice
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	if result.AdvisorModel != "" {
		payload["advisor_model"] = result.AdvisorModel
	}
	return payload
}
