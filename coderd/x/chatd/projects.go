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

// DeleteChatProjectWithoutEvents marks a project deleted, which hides every
// user's chats in it until dbpurge removes them, and returns those chats.
// Callers must have authorized deleting the project.
func DeleteChatProjectWithoutEvents(ctx context.Context, db database.Store, projectID uuid.UUID) ([]database.Chat, error) {
	//nolint:gocritic // Sharees own some of the chats; the caller authorized deleting the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	err := db.InTx(func(tx database.Store) error {
		chats, err := tx.GetChatProjectChatFamilies(chatdCtx, projectID)
		if err != nil {
			return xerrors.Errorf("get project chats: %w", err)
		}
		chatIDs := make([]uuid.UUID, 0, len(chats))
		for _, chat := range chats {
			chatIDs = append(chatIDs, chat.ID)
		}
		// Dropping the leases stops running chats, and acquisition skips
		// chats of deleted projects so they are not picked up again.
		if err := tx.DeleteChatHeartbeatsByChatIDs(chatdCtx, chatIDs); err != nil {
			return xerrors.Errorf("release project chat leases: %w", err)
		}
		if err := tx.MarkChatProjectDeleted(ctx, projectID); err != nil {
			return xerrors.Errorf("mark project deleted: %w", err)
		}
		deleted = chats
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// ChatProjectUsableBy ignores role grants because project chats read and
// write memory as chatd.
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
