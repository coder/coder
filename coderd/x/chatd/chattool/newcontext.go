package chattool

import (
	"context"

	"charm.land/fantasy"
)

const (
	// NewContextToolName names the tool that asks chatd to start a fresh
	// context window.
	NewContextToolName = "new_context"
	// NewContextScheduledStatus marks a new_context result that chatd
	// honors as a rollover request.
	NewContextScheduledStatus = "scheduled"
)

// NewContextOptions configures the new_context tool.
type NewContextOptions struct {
	// RolloverDisabled is set when the compaction threshold is 100%. The
	// tool stays registered then so the tool block does not change, but
	// it refuses.
	RolloverDisabled bool
}

// NewContext returns a tool whose successful result is the durable
// receipt of a rollover request. chatd honors it once every sibling tool
// call of the step has settled.
func NewContext(opts NewContextOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(NewContextToolName,
		"Start a new context window (the workspace is untouched). "+
			"First save a checkpoint with "+SaveCheckpointToolName+": the new window has no transcript or summary, only that checkpoint and chat_history. "+
			"It starts once this step settles (sibling calls still complete). Honored once per window; refused when automatic rollover is off (threshold 100%).",
		func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if opts.RolloverDisabled {
				return toolResponse(map[string]any{
					"code":  "rollover_disabled",
					"error": "Automatic rollover is disabled (compaction threshold 100%), so no new context window can start. Continue in this window.",
				}), nil
			}
			return toolResponse(map[string]any{
				"status":  NewContextScheduledStatus,
				"message": "A new context window will start without summarizing conversation history.",
			}), nil
		})
}
