package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// DeleteChatProject deletes a project and publishes events for its chats.
// See [chatstate.DeleteChatProject].
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := chatstate.DeleteChatProject(ctx, p.db, p.pubsub, projectID)
	if err != nil && deleted == nil {
		return nil, err
	}
	if err != nil {
		p.logger.Warn(ctx, "publish chat:update for deleted project chats failed; their runners stop at the next heartbeat renewal",
			slog.F("project_id", projectID), slog.Error(err))
	}
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, nil
}

// ChatProjectUsableBy reports whether userID owns project or holds a grant
// in its ACL. Role grants do not count, because the chat would read memory
// the owner did not share. Memory tools run as chatd, so callers must use
// this check, not dbauthz.
func ChatProjectUsableBy(ctx context.Context, db database.Store, project database.ChatProject, userID uuid.UUID) (bool, error) {
	if project.OwnerID == userID {
		return true, nil
	}
	//nolint:gocritic // The check answers for the chat owner, not the caller.
	usable, err := db.IsChatProjectAccessibleByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectAccessibleByUserIDParams{
		ProjectID: project.ID,
		UserID:    userID,
	})
	if err != nil {
		return false, xerrors.Errorf("check chat project access: %w", err)
	}
	return usable, nil
}
