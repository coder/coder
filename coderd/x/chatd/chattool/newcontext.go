package chattool

import (
	"context"
	"time"

	"charm.land/fantasy"
)

const (
	// NewContextToolName ends the current context window. The server
	// commits the window boundary in the same step as the tool result.
	NewContextToolName = "new_context"
	// NewContextSuccessText is the result text of a call that ends the
	// window. chatd keys the boundary on a non-error result.
	NewContextSuccessText = "A new context window starts after this step."
)

const newContextDescription = "Start a new context window. First save your checkpoint with notes: " +
	"the new window has no earlier messages, only your notes and chat_history. " +
	"The window ends after this step. Call it alone."

// NewContext returns the new_context tool. It refuses until a note
// changed after windowStart, so the agent cannot drop its task state.
func NewContext(store NotesStore, windowStart time.Time) fantasy.AgentTool {
	return fantasy.NewAgentTool(NewContextToolName, newContextDescription,
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			changed, err := store.ChangedSince(ctx, windowStart)
			switch {
			case err != nil:
				return fantasy.NewTextErrorResponse("failed to read notes"), nil
			case !changed:
				return fantasy.NewTextErrorResponse("Save your checkpoint with notes first."), nil
			}
			return fantasy.NewTextResponse(NewContextSuccessText), nil
		})
}
