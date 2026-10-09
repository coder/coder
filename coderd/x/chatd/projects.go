package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// DeleteChatProject deletes a project and publishes events for its chats.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := DeleteChatProjectWithoutEvents(ctx, p.db, projectID)
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, err
}

// DeleteChatProjectWithoutEvents tombstones a project and archives every
// user's chats in it with their sub-chats, then returns those chats. It
// clears their leases, so running generations in the project, other users'
// included, stop at their next heartbeat renewal. dbpurge deletes the rows
// later. Callers must have authorized deleting the project.
func DeleteChatProjectWithoutEvents(ctx context.Context, db database.Store, projectID uuid.UUID) ([]database.Chat, error) {
	//nolint:gocritic // Sharees own some of the chats; the caller authorized deleting the project.
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
		if err := tx.LockChatProjectRootChats(chatdCtx, projectID); err != nil {
			return xerrors.Errorf("lock project chats: %w", err)
		}
		chats, err := tx.GetChatProjectChatFamilies(chatdCtx, projectID)
		if err != nil {
			return xerrors.Errorf("get project chats: %w", err)
		}
		chatIDs := make([]uuid.UUID, 0, len(chats))
		for _, chat := range chats {
			chatIDs = append(chatIDs, chat.ID)
		}
		if err := tx.DeleteChatQueuedMessagesByChatIDs(chatdCtx, chatIDs); err != nil {
			return xerrors.Errorf("clear project chat queues: %w", err)
		}
		if err := tx.ArchiveChatsOfDeletedChatProject(chatdCtx, chatIDs); err != nil {
			return xerrors.Errorf("archive project chats: %w", err)
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
