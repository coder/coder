package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// DeleteChatProject deletes a project and publishes hard_deleted watch
// events for the chats it deleted. See DeleteChatProjectWithoutEvents.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := DeleteChatProjectWithoutEvents(ctx, p.db, projectID)
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, err
}

// DeleteChatProjectWithoutEvents deletes a project together with its root
// chats, every user's, and their sub-chats, in one transaction, and
// returns the chats it deleted. Running chats are deleted too: their
// workers stop when the next heartbeat finds the lease gone. Callers must
// have authorized deleting the project.
func DeleteChatProjectWithoutEvents(ctx context.Context, db database.Store, projectID uuid.UUID) ([]database.Chat, error) {
	//nolint:gocritic // Sharees own some of the chats; the caller authorized deleting the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	err := database.InChatProjectDeleteTx(chatdCtx, db, projectID, func(tx database.Store, rootIDs, chatIDs []uuid.UUID) error {
		// InChatProjectDeleteTx reruns this on deadlock.
		deleted = nil
		if len(rootIDs) > 0 {
			var err error
			deleted, err = tx.GetChatsByIDs(chatdCtx, chatIDs)
			if err != nil {
				return xerrors.Errorf("get project chats: %w", err)
			}
			if err := tx.DeleteChatFamiliesByRootIDs(chatdCtx, rootIDs); err != nil {
				return xerrors.Errorf("delete project chats: %w", err)
			}
		}
		if err := tx.DeleteChatProjectByID(ctx, projectID); err != nil {
			return xerrors.Errorf("delete project: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

// ChatProjectUsableBy reports whether userID may run chats in project: the
// owner always may, and others need a read grant in the project ACL. Role
// grants, such as an administrator's, do not count, because a chat in a
// project reads and writes memory its owner was not given. Memory tools run
// as chatd, so this is the check that honors revoked and disabled sharing.
func ChatProjectUsableBy(ctx context.Context, db database.Store, project database.ChatProject, userID uuid.UUID) (bool, error) {
	if project.OwnerID == userID {
		return true, nil
	}
	if rbac.ChatACLDisabled() {
		return false, nil
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
