package chatd

import (
	"context"
	"database/sql"
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

func (p *Server) cleanupDeletedChatProject(ctx context.Context, projectID uuid.UUID) error {
	//nolint:gocritic // The cleanup archives every user's chats in the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	archived, chats, err := chatstate.ArchiveDeletedChatProject(chatdCtx, p.db, p.pubsub, projectID)
	if errors.Is(err, chatstate.ErrChatUpdatePublishFailed) {
		p.logger.Warn(ctx, "publish chat:update for deleted project chats failed; their runners stop at the next heartbeat renewal",
			slog.F("project_id", projectID), slog.Error(err))
	} else if err != nil {
		return err
	}
	p.scheduleArchiveDebugCleanup(ctx, archived)
	// Chats archived before the delete need the event too.
	p.publishChatPubsubEvents(chats, codersdk.ChatWatchEventKindProjectDeleted)
	return nil
}

// usableChatProject loads the project and reports whether the chat owner can
// use it. It reports false when the project is missing, the lookup or access
// check fails, or the owner cannot use it, and logs every case except a
// missing project.
func (p *Server) usableChatProject(ctx context.Context, logger slog.Logger, projectID uuid.UUID, chat database.Chat) (database.ChatProject, bool) {
	logger = logger.With(slog.F("project_id", projectID))
	project, err := p.db.GetChatProjectByID(ctx, projectID)
	if err != nil {
		// A project deleted mid-turn is expected.
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "failed to load chat project, turn runs without project memory or instructions", slog.Error(err))
		}
		return database.ChatProject{}, false
	}
	usable, err := ChatProjectUsableBy(ctx, p.db, project, chat.OwnerID)
	if err != nil {
		logger.Warn(ctx, "failed to check chat project access, turn runs without project memory or instructions", slog.Error(err))
		return database.ChatProject{}, false
	}
	if !usable {
		logger.Debug(ctx, "chat owner cannot use chat project, turn runs without project memory or instructions",
			slog.F("project_owner_id", project.OwnerID))
		return database.ChatProject{}, false
	}
	return project, true
}

// ChatProjectUsableBy reports whether userID owns project or has an ACL
// grant on it. Role grants do not count, because the chat would read memory
// and instructions the project owner did not share. Memory tools and
// instruction loading run as chatd, so they must use this check, not dbauthz.
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
