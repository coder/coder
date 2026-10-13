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

// ScheduleDeletedChatProjectCleanup archives a deleted project's chats in
// the background so the delete request does not wait for, or fail on, the
// chat locks. The worker's sweep retries a cleanup that fails.
func (p *Server) ScheduleDeletedChatProjectCleanup(ctx context.Context, projectID uuid.UUID) {
	cleanupCtx, stop := p.inflightContext(ctx)
	err := p.goInflight(func() {
		defer stop()
		if err := p.cleanupDeletedChatProject(cleanupCtx, projectID); err != nil && cleanupCtx.Err() == nil {
			p.logger.Warn(cleanupCtx, "deleted chat project cleanup failed; the sweep retries it",
				slog.F("project_id", projectID), slog.Error(err))
		}
	})
	if err != nil {
		stop()
	}
}

// cleanupDeletedChatProject runs [chatstate.ArchiveDeletedChatProject],
// publishes a removed watch event per archived chat, and schedules their
// debug-data cleanup.
func (p *Server) cleanupDeletedChatProject(ctx context.Context, projectID uuid.UUID) error {
	//nolint:gocritic // The cleanup archives every user's chats in the project.
	chats, err := chatstate.ArchiveDeletedChatProject(dbauthz.AsChatd(ctx), p.db, p.pubsub, projectID)
	if errors.Is(err, chatstate.ErrChatUpdatePublishFailed) {
		p.logger.Warn(ctx, "publish chat:update for deleted project chats failed; their runners stop at the next heartbeat renewal",
			slog.F("project_id", projectID), slog.Error(err))
	} else if err != nil {
		return err
	}
	p.publishChatPubsubEvents(chats, codersdk.ChatWatchEventKindRemoved)
	p.scheduleArchiveDebugCleanup(ctx, chats)
	return nil
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
