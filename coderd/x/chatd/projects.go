package chatd

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// DeleteChatProject runs [chatstate.DeleteChatProject], publishes a removed
// watch event per chat, and schedules the chats' debug-data cleanup. It
// returns a nil error once the delete commits; a failed chat:update
// publish is logged, and those chats' runners stop at their next heartbeat
// renewal.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) (chatstate.DeletedChatProject, error) {
	deleted, err := chatstate.DeleteChatProject(ctx, p.db, p.pubsub, projectID)
	if errors.Is(err, chatstate.ErrChatUpdatePublishFailed) {
		p.logger.Warn(ctx, "publish chat:update for deleted project chats failed; their runners stop at the next heartbeat renewal",
			slog.F("project_id", projectID), slog.Error(err))
	} else if err != nil {
		return chatstate.DeletedChatProject{}, err
	}
	p.publishChatPubsubEvents(deleted.Archived, codersdk.ChatWatchEventKindRemoved)
	p.scheduleArchiveDebugCleanup(ctx, deleted.Archived)
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
