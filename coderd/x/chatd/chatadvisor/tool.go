package chatadvisor

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/codersdk"
)

// ToolName is the identifier the advisor tool registers under. The parent
// agent's exclusive-tool policy and the advisor-guidance block both reference
// this name, so keeping them synchronized requires a single source of truth.
const ToolName = "advisor"

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
		ToolName,
		"Ask a separate advisor pass for strategic guidance about planning, architecture, tradeoffs, or debugging strategy. Provide a brief question of 2000 runes or fewer, summarizing context instead of pasting long logs or transcripts. The advisor sees recent conversation context, runs without tools for a single step, and responds to the parent agent rather than the end user.",
		func(ctx context.Context, args AdvisorArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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

			// The publisher is injected into the execution context by
			// chatloop.ExecuteLocalTools; a missing publisher indicates
			// a wiring bug rather than a recoverable condition, so fail
			// loudly instead of silently dropping the streamed advice.
			publish := chatloop.MessagePartPublisherFromContext(ctx)
			if publish == nil {
				return fantasy.NewTextErrorResponse("advisor tool requires a message-part publisher on the context; this is an internal tool bug"), nil
			}

			var runOpts *RunAdvisorOptions
			if call.ID != "" {
				runOpts = &RunAdvisorOptions{
					OnAdviceDelta: func(delta string) {
						if delta == "" {
							return
						}
						publish(codersdk.ChatMessageRoleTool, codersdk.ChatMessagePart{
							Type:        codersdk.ChatMessagePartTypeToolResult,
							ToolCallID:  call.ID,
							ToolName:    ToolName,
							ResultDelta: delta,
						})
					},
					OnAdviceReset: func() {
						publish(codersdk.ChatMessageRoleTool, codersdk.ChatMessagePart{
							Type:        codersdk.ChatMessagePartTypeToolResult,
							ToolCallID:  call.ID,
							ToolName:    ToolName,
							ResultReset: true,
						})
					},
				}
			}

			result, err := opts.Runtime.RunAdvisor(ctx, question, opts.GetConversationSnapshot(), runOpts)
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
