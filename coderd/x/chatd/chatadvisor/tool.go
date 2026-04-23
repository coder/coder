package chatadvisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/fantasy"
)

// advisorQuestionMaxRunes caps the parent agent's question at a length
// that leaves room in the advisor prompt for system preamble and recent
// conversation context.
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
		"Ask a separate advisor pass for strategic guidance about planning, architecture, tradeoffs, or debugging strategy. Provide a brief question. The advisor sees recent conversation context, runs without tools for a single step, and responds to the parent agent rather than the end user.",
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
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("question must be %d runes or fewer", advisorQuestionMaxRunes),
				), nil
			}

			result, err := opts.Runtime.RunAdvisor(ctx, question, opts.GetConversationSnapshot())
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return fantasy.NewTextResponse("{}"), nil
			}
			return fantasy.NewTextResponse(string(data)), nil
		},
	)
}
