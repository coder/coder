package chatd

import (
	"context"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func buildAssistantPartsForPersist(
	ctx context.Context,
	logger slog.Logger,
	assistantBlocks []fantasy.Content,
	toolResults []fantasy.ToolResultContent,
	step chatloop.PersistedStep,
	toolNameToConfigID map[string]uuid.UUID,
) []codersdk.ChatMessagePart {
	parts := make([]codersdk.ChatMessagePart, 0, len(assistantBlocks)+len(toolResults))
	// reasoningIdx walks reasoning blocks in occurrence order so we
	// can apply the matching ReasoningStartedAt/ReasoningCompletedAt
	// entry from step onto each reasoning part's CreatedAt and
	// CompletedAt.
	reasoningIdx := 0
	for _, block := range assistantBlocks {
		part := chatprompt.PartFromContentWithLogger(ctx, logger, block)
		if part.ToolName != "" {
			if configID, ok := toolNameToConfigID[part.ToolName]; ok {
				part.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
			}
		}
		if part.Type == codersdk.ChatMessagePartTypeToolCall && part.ToolCallID != "" && step.ToolCallCreatedAt != nil {
			if ts, ok := step.ToolCallCreatedAt[part.ToolCallID]; ok {
				part.CreatedAt = &ts
			}
		}
		if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID != "" && step.ToolResultCreatedAt != nil {
			if ts, ok := step.ToolResultCreatedAt[part.ToolCallID]; ok {
				part.CreatedAt = &ts
			}
		}
		if part.Type == codersdk.ChatMessagePartTypeReasoning {
			if reasoningIdx < len(step.ReasoningStartedAt) {
				if ts := step.ReasoningStartedAt[reasoningIdx]; !ts.IsZero() {
					part.CreatedAt = &ts
				}
			}
			if reasoningIdx < len(step.ReasoningCompletedAt) {
				if ts := step.ReasoningCompletedAt[reasoningIdx]; !ts.IsZero() {
					part.CompletedAt = &ts
				}
			}
			reasoningIdx++
		}
		parts = append(parts, part)
	}
	for _, tr := range toolResults {
		attachments, err := chattool.AttachmentsFromMetadata(tr.ClientMetadata)
		if err != nil {
			logger.Warn(ctx, "skipping malformed tool attachment metadata",
				slog.F("tool_name", tr.ToolName),
				slog.F("tool_call_id", tr.ToolCallID),
				slog.Error(err),
			)
			continue
		}
		for _, attachment := range attachments {
			parts = append(parts, codersdk.ChatMessageFile(
				attachment.FileID,
				attachment.MediaType,
				attachment.Name,
			))
		}
	}
	return parts
}
