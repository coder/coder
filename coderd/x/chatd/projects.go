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

// ErrChatProjectHasActiveChats reports that a project could not be deleted
// because a worker holds one of its chats.
var ErrChatProjectHasActiveChats = xerrors.New("chat project has running chats")

// DeleteChatProject deletes a project together with its root chats, every
// user's, and their sub-chats. It fails with ErrChatProjectHasActiveChats
// while a worker holds any of them, so in-flight turns are never cut off.
// Callers must have authorized deleting the project.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) error {
	// The project's chats belong to its sharees too, so the deletion runs as
	// chatd, which can delete any chat.
	//nolint:gocritic // The caller authorized deleting the project; see above.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	err := p.db.InTx(func(tx database.Store) error {
		chats, err := tx.GetChatProjectChatsForDelete(chatdCtx, projectID)
		if err != nil {
			return xerrors.Errorf("lock project chats: %w", err)
		}
		for _, chat := range chats {
			// A chat is mid-turn while a worker holds it. Queued chats
			// have no worker yet, and the row locks keep one from
			// acquiring them before the delete commits.
			if chat.WorkerID.Valid {
				return ErrChatProjectHasActiveChats
			}
		}
		if err := tx.DeleteChatProjectChats(chatdCtx, projectID); err != nil {
			return xerrors.Errorf("delete project chats: %w", err)
		}
		if err := tx.DeleteChatProjectByID(ctx, projectID); err != nil {
			return xerrors.Errorf("delete project: %w", err)
		}
		deleted = chats
		return nil
	}, nil)
	if err != nil {
		return err
	}
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindDeleted)
	return nil
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
