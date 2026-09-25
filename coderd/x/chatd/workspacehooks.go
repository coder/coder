package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// workspaceHookResults collects the model_context and user_message
// that workspace hooks attached to tool results in a step, so they can
// become transcript rows the same way deployment hook results do: model
// context as a model-only row, user message as a user-visible notice.
// The tool result text itself never carries model_context.
// PROTOTYPE (CODAGT-1083).
func workspaceHookResults(ctx context.Context, logger slog.Logger, content []fantasy.Content) []*chathooks.Result {
	var results []*chathooks.Result
	for _, block := range content {
		toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if !ok {
			continue
		}
		decisions, err := chattool.WorkspaceHooksFromMetadata(toolResult.ClientMetadata)
		if err != nil {
			logger.Warn(ctx, "skipping malformed workspace hook metadata",
				slog.F("tool_name", toolResult.ToolName),
				slog.F("tool_call_id", toolResult.ToolCallID),
				slog.Error(err),
			)
			continue
		}
		for _, decision := range decisions {
			if strings.TrimSpace(decision.ModelContext) == "" && strings.TrimSpace(decision.UserMessage) == "" {
				continue
			}
			results = append(results, &chathooks.Result{
				ModelContext: decision.ModelContext,
				UserMessage:  decision.UserMessage,
			})
		}
	}
	return results
}
