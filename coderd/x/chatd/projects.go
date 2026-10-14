package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// ChatProjectUsableBy reports whether userID owns project or has an ACL
// grant on it. Role grants do not count, because the chat would read memory
// and instructions the project owner did not share. Memory tools and
// instruction loading run as chatd, so they must use this check, not dbauthz.
// usableChatProject loads the project and reports whether the chat owner can
// use it. Lookup failures and denials are logged and report false.
func (p *Server) usableChatProject(ctx context.Context, logger slog.Logger, projectID uuid.UUID, chat database.Chat) (database.ChatProject, bool) {
	logger = logger.With(slog.F("chat_id", chat.ID), slog.F("project_id", projectID))
	project, err := p.db.GetChatProjectByID(ctx, projectID)
	if err != nil {
		logger.Warn(ctx, "failed to load chat project", slog.Error(err))
		return database.ChatProject{}, false
	}
	usable, err := ChatProjectUsableBy(ctx, p.db, project, chat.OwnerID)
	if err != nil {
		logger.Warn(ctx, "failed to check chat project access", slog.Error(err))
		return database.ChatProject{}, false
	}
	if !usable {
		logger.Debug(ctx, "chat owner cannot use chat project, memory and instructions disabled",
			slog.F("owner_id", chat.OwnerID), slog.F("project_owner_id", project.OwnerID))
		return database.ChatProject{}, false
	}
	return project, true
}

func ChatProjectUsableBy(ctx context.Context, db database.Store, project database.ChatProject, userID uuid.UUID) (bool, error) {
	if project.OwnerID == userID {
		return true, nil
	}
	//nolint:gocritic // The check answers for the chat owner, not the caller.
	usable, err := db.IsChatProjectUsableByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectUsableByUserIDParams{
		ProjectID: project.ID,
		UserID:    userID,
	})
	if err != nil {
		return false, xerrors.Errorf("check chat project access: %w", err)
	}
	return usable, nil
}
