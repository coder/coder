package chatd

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// resolveChatProjectID returns the project the chat belongs to, or an
// invalid ID when it has none or chat projects are disabled. Subagent chats
// do not carry a project, so they inherit the root chat's project at the cost
// of one extra lookup. A failed lookup is logged and treated as no project.
func (p *Server) resolveChatProjectID(ctx context.Context, logger slog.Logger, chat database.Chat) uuid.NullUUID {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return uuid.NullUUID{}
	}
	if chat.ProjectID.Valid || !chat.RootChatID.Valid || chat.RootChatID.UUID == chat.ID {
		return chat.ProjectID
	}
	root, err := p.db.GetChatByID(ctx, chat.RootChatID.UUID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "failed to load root chat for chat project",
				slog.F("root_chat_id", chat.RootChatID.UUID),
				slog.Error(err),
			)
		}
		return uuid.NullUUID{}
	}
	return root.ProjectID
}

// resolveUsableChatProjectID returns the chat's project, as resolved by
// resolveChatProjectID, when the chat owner can use it. Otherwise it returns
// an invalid ID, and the chat runs without the project's instructions,
// skills, and memory.
func (p *Server) resolveUsableChatProjectID(ctx context.Context, logger slog.Logger, chat database.Chat) uuid.NullUUID {
	projectID := p.resolveChatProjectID(ctx, logger, chat)
	if !projectID.Valid {
		return uuid.NullUUID{}
	}
	if _, ok := p.usableChatProject(ctx, logger, projectID.UUID, chat); !ok {
		return uuid.NullUUID{}
	}
	return projectID
}

// loadProjectInstructions returns the instructions block of a project the
// chat owner can use, as checked by resolveUsableChatProjectID, or "" when it
// has none or they cannot be loaded. Instructions are read on every turn so
// edits reach existing chats on their next turn.
func (p *Server) loadProjectInstructions(ctx context.Context, logger slog.Logger, projectID uuid.UUID) string {
	row, err := p.db.GetChatProjectInstructionsByProjectID(ctx, projectID)
	if err != nil {
		// A project without a row has no instructions.
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "failed to load chat project instructions",
				slog.F("project_id", projectID),
				slog.Error(err),
			)
		}
		return ""
	}
	return formatProjectInstructions(row.ChatProjectInstruction.Instructions)
}

// projectInstructionsTagPattern matches opening and closing
// <project-instructions> tags, including case and spacing variants.
var projectInstructionsTagPattern = regexp.MustCompile(`(?i)<\s*/?\s*project-instructions\b[^>]*>`)

// formatProjectInstructions wraps project instructions for the system
// prompt. The text is sanitized again because rows may predate the API's
// sanitization or be written outside it. Wrapper tags inside the text are
// escaped so the text cannot end the block early and place content outside
// it.
func formatProjectInstructions(instructions string) string {
	trimmed := strings.TrimSpace(codersdk.SanitizePromptText(instructions))
	if trimmed == "" {
		return ""
	}
	trimmed = projectInstructionsTagPattern.ReplaceAllStringFunc(trimmed, func(tag string) string {
		return "&lt;" + tag[1:len(tag)-1] + "&gt;"
	})
	return "The following instructions were set for the project this chat belongs to and apply to every chat in it.\n" +
		"<project-instructions>\n" + trimmed + "\n</project-instructions>"
}
