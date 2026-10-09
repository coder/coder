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
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	buffer := chatstate.NewPublishBuffer(p.pubsub)
	defer buffer.Discard()
	deleted, err := deleteChatProject(ctx, p.db, buffer, projectID)
	if err != nil {
		return nil, err
	}
	// The chat:update messages stop the archived chats' runners right away.
	if err := buffer.Flush(); err != nil {
		p.logger.Warn(ctx, "failed to publish chat updates for deleted chat project", slog.F("project_id", projectID), slog.Error(err))
	}
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, nil
}

// DeleteChatProjectWithoutEvents tombstones a project and archives every
// user's chats in it with their sub-chats, then returns those chats as they
// were before the archive. It clears their worker and runner, so running
// generations in the project, other users' included, stop at their next
// heartbeat renewal. Chat retention deletes the rows later. ctx's actor must
// be allowed to delete the project: the tombstone update runs as that actor
// and fails otherwise, and the chat reads and writes run as chatd.
func DeleteChatProjectWithoutEvents(ctx context.Context, db database.Store, projectID uuid.UUID) ([]database.Chat, error) {
	buffer := chatstate.NewPublishBuffer(nil)
	defer buffer.Discard()
	return deleteChatProject(ctx, db, buffer, projectID)
}

func deleteChatProject(ctx context.Context, db database.Store, buffer *chatstate.PublishBuffer, projectID uuid.UUID) ([]database.Chat, error) {
	//nolint:gocritic // Sharees own some of the chats; the tombstone update authorizes the delete.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	err := db.InTx(func(tx database.Store) error {
		// FOR UPDATE waits for root chat creations in the project, which
		// hold it FOR SHARE, and makes a concurrent delete find no project.
		if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
			return xerrors.Errorf("lock project: %w", err)
		}
		// Runs as the caller: this call is the delete authorization check.
		if err := tx.UpdateChatProjectDeletedByID(ctx, projectID); err != nil {
			return xerrors.Errorf("mark project deleted: %w", err)
		}
		chats, err := chatstate.ArchiveDeletedChatProject(chatdCtx, tx, buffer, projectID)
		if err != nil {
			return err
		}
		deleted = chats
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
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
