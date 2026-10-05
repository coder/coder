package chatd

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// resolveProjectInstructions returns the system prompt block holding the
// instructions of the chat's project, or "" when there are none. Subagent
// chats do not carry a project, so they inherit the root chat's project at
// the cost of one extra lookup. Instructions are read on every turn so edits
// reach existing chats on their next turn.
//
// Lookups fail open: a chat keeps working without project instructions when
// they cannot be loaded.
func (p *Server) resolveProjectInstructions(ctx context.Context, logger slog.Logger, chat database.Chat) string {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return ""
	}
	projectID := chat.ProjectID
	if !projectID.Valid && chat.RootChatID.Valid && chat.RootChatID.UUID != chat.ID {
		root, err := p.db.GetChatByID(ctx, chat.RootChatID.UUID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				logger.Warn(ctx, "failed to load root chat for project instructions",
					slog.F("root_chat_id", chat.RootChatID.UUID),
					slog.Error(err),
				)
			}
			return ""
		}
		projectID = root.ProjectID
	}
	if !projectID.Valid {
		return ""
	}

	row, err := p.db.GetChatProjectInstructionsByProjectID(ctx, projectID.UUID)
	if err != nil {
		// A project without a row has no instructions.
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "failed to load chat project instructions",
				slog.F("project_id", projectID.UUID),
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
