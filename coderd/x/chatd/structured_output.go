package chatd

import (
	"context"
	"encoding/json"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/structuredoutput"
	"github.com/coder/coder/v2/codersdk"
)

// structuredOutputOverlayPrompt instructs the model how to finish a
// structured output turn. Injected only when a structured output
// request is active.
const structuredOutputOverlayPrompt = `<structured_output>
This turn requires a structured final answer.
- Use your normal tools first as needed to gather information and complete the task.
- When the work is done, call the ` + structuredoutput.ToolName + ` tool exactly once with the final answer in its "output" argument. The output must satisfy the tool's JSON schema.
- Never emit the final answer as plain text; only the ` + structuredoutput.ToolName + ` tool result counts.
- Call ` + structuredoutput.ToolName + ` alone, never batched with other tool calls.
- If the tool returns a validation error, fix the listed fields and call it again.
</structured_output>`

// ExtractStructuredOutputValue returns the validated structured
// output value from the active turn's persisted history: the Result
// of the latest successful (non-error) coder_structured_output tool
// result after the turn's trigger user message. The bool reports
// whether such a result exists. This pins the recovery contract for
// SDK clients: the tool-result part's Result field carries the
// canonical validated JSON of the unwrapped "output" value.
func ExtractStructuredOutputValue(messages []database.ChatMessage) (json.RawMessage, bool, error) {
	start := 0
	for i, msg := range messages {
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleUser {
			start = i + 1
		}
	}
	for i := len(messages) - 1; i >= start; i-- {
		msg := messages[i]
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return nil, false, xerrors.Errorf("parse tool message: %w", err)
		}
		for j := len(parts) - 1; j >= 0; j-- {
			part := parts[j]
			if part.Type != codersdk.ChatMessagePartTypeToolResult ||
				part.ToolName != structuredoutput.ToolName ||
				part.IsError {
				continue
			}
			return part.Result, true, nil
		}
	}
	return nil, false, nil
}

// activeTurnResponseFormat returns the structured output request for
// the active turn, if any. It walks backward over the full
// generation-state message list (not compaction-filtered prompt
// rows) to the latest non-deleted, non-compressed user message and
// parses its response-format part. Compaction rows are all
// Compressed=true, so the trigger user message stays discoverable
// after compaction. When a message somehow carries multiple
// response-format parts, the last one wins.
func activeTurnResponseFormat(
	ctx context.Context,
	logger slog.Logger,
	messages []database.ChatMessage,
) *structuredoutput.Request {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Deleted || message.Compressed || message.Role != database.ChatMessageRoleUser {
			continue
		}
		// Only user-authored (user-visible) messages carry the
		// response-format part. Skip hidden model-visibility user
		// rows (e.g. injected context) like
		// activeTurnAPIKeyIDFromMessages does.
		if !isUserVisibleChatMessage(message) {
			continue
		}
		parts, err := chatprompt.ParseContent(message)
		if err != nil {
			logger.Warn(ctx, "failed to parse user message for response format",
				slog.F("message_id", message.ID),
				slog.Error(err),
			)
			return nil
		}
		var format *codersdk.ChatResponseFormat
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeResponseFormat && part.ResponseFormat != nil {
				format = part.ResponseFormat
			}
		}
		if format == nil {
			return nil
		}
		request, verr := structuredoutput.NewRequest(format)
		if verr != nil {
			// The HTTP layer validates before persisting, so a
			// persisted-but-invalid format indicates a version skew
			// or manual edit. Degrade to a normal turn rather than
			// failing generation forever.
			logger.Warn(ctx, "persisted response format is invalid; ignoring",
				slog.F("message_id", message.ID),
				slog.F("field", verr.Field),
				slog.F("detail", verr.Detail),
			)
			return nil
		}
		return request
	}
	return nil
}
