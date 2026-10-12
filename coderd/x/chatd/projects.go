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

// DeleteChatProject runs [chatstate.DeleteChatProject] for callers without a
// [Server]. It returns a nil error once the delete commits; a failed
// chat:update publish is logged, and those chats' runners stop at their
// next heartbeat renewal.
func DeleteChatProject(ctx context.Context, logger slog.Logger, db database.Store, publisher chatstate.Publisher, projectID uuid.UUID) (chatstate.DeletedChatProject, error) {
	deleted, err := chatstate.DeleteChatProject(ctx, db, publisher, projectID)
	if errors.Is(err, chatstate.ErrChatUpdatePublishFailed) {
		logger.Warn(ctx, "publish chat:update for deleted project chats failed; their runners stop at the next heartbeat renewal",
			slog.F("project_id", projectID), slog.Error(err))
		return deleted, nil
	}
	return deleted, err
}

// DeleteChatProject runs the package-level [DeleteChatProject], publishes a
// removed watch event per chat, and schedules the chats' debug-data
// cleanup.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) (chatstate.DeletedChatProject, error) {
	deleted, err := DeleteChatProject(ctx, p.logger, p.db, p.pubsub, projectID)
	if err != nil {
		return chatstate.DeletedChatProject{}, err
	}
	p.publishChatPubsubEvents(deleted.After, codersdk.ChatWatchEventKindRemoved)
	p.scheduleArchiveDebugCleanup(ctx, deleted.After)
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
