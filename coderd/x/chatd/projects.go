package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// DeleteChatProject deletes a project with its chats and publishes events.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := DeleteChatProjectWithoutEvents(ctx, p.db, projectID)
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, err
}

// DeleteChatProjectWithoutEvents deletes a project with every user's chats
// in it. Callers must have authorized deleting the project.
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
		var rootIDs []uuid.UUID
		for _, chat := range chats {
			chatIDs = append(chatIDs, chat.ID)
			if !chat.ParentChatID.Valid {
				rootIDs = append(rootIDs, chat.ID)
			}
		}
		// Messages go first: the chat delete locks heartbeat rows, which
		// lease renewal waits on, until commit.
		if err := tx.DeleteChatMessagesByChatIDs(chatdCtx, chatIDs); err != nil {
			return xerrors.Errorf("delete project chat messages: %w", err)
		}
		if err := tx.DeleteChatFamiliesByRootIDs(chatdCtx, rootIDs); err != nil {
			return xerrors.Errorf("delete project chats: %w", err)
		}
		if err := tx.DeleteChatProjectByID(ctx, projectID); err != nil {
			return xerrors.Errorf("delete project: %w", err)
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
