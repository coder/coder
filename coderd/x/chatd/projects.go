package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// ErrChatProjectHasRunningChats reports that a project could not be
// deleted because a live worker holds one of its chats.
var ErrChatProjectHasRunningChats = xerrors.New("chat project has running chats")

// chatProjectDeleteBatchSize bounds the root chats, with their sub-chats,
// deleted per transaction, so a large project does not hold its rows
// locked for one long transaction.
const chatProjectDeleteBatchSize = 100

// DeleteChatProject deletes a project and publishes deleted watch events
// for its chats. See DeleteChatProjectWithChats.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := DeleteChatProjectWithChats(ctx, p.db, projectID, p.inFlightChatStaleAfter)
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindDeleted)
	return deleted, err
}

// DeleteChatProjectWithChats deletes a project together with its root
// chats, every user's, and their sub-chats, and returns the chats it
// deleted. It fails with ErrChatProjectHasRunningChats while a live worker
// holds any of them, so in-flight turns are never cut off; a lease whose
// heartbeat is older than staleAfter does not count. Chats go in batches,
// so a failure can leave the project with some chats already deleted;
// retrying finishes the job. Callers must have authorized deleting the
// project.
func DeleteChatProjectWithChats(ctx context.Context, db database.Store, projectID uuid.UUID, staleAfter time.Duration) ([]database.Chat, error) {
	// The project's chats belong to its sharees too, so chats are deleted as
	// chatd, which can delete any chat.
	//nolint:gocritic // The caller authorized deleting the project; see above.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	for {
		var done bool
		err := db.InTx(func(tx database.Store) error {
			// Locking the project blocks new chats from joining it until
			// this batch commits.
			if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
				return xerrors.Errorf("lock project: %w", err)
			}
			locked, err := tx.LockChatProjectChatsForDelete(chatdCtx, database.LockChatProjectChatsForDeleteParams{
				ProjectID:  projectID,
				LimitCount: chatProjectDeleteBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("lock project chats: %w", err)
			}
			if len(locked) == 0 {
				if err := tx.DeleteChatProjectByID(ctx, projectID); err != nil {
					return xerrors.Errorf("delete project: %w", err)
				}
				done = true
				return nil
			}
			ids := make([]uuid.UUID, 0, len(locked))
			for _, row := range locked {
				running, err := chatHeldByLiveWorker(chatdCtx, tx, row, staleAfter)
				if err != nil {
					return err
				}
				if running {
					return ErrChatProjectHasRunningChats
				}
				ids = append(ids, row.ID)
			}
			chats, err := tx.GetChatsByIDs(chatdCtx, ids)
			if err != nil {
				return xerrors.Errorf("get project chats: %w", err)
			}
			if err := tx.DeleteChatsByIDs(chatdCtx, ids); err != nil {
				return xerrors.Errorf("delete project chats: %w", err)
			}
			deleted = append(deleted, chats...)
			return nil
		}, nil)
		if err != nil || done {
			return deleted, err
		}
	}
}

// chatHeldByLiveWorker reports whether a worker with a fresh heartbeat holds
// the chat. A turn can end with worker_id still set, and a replica that dies
// then never clears it, so worker_id alone would block deletion forever.
func chatHeldByLiveWorker(ctx context.Context, db database.Store, row database.LockChatProjectChatsForDeleteRow, staleAfter time.Duration) (bool, error) {
	if !row.WorkerID.Valid || !row.RunnerID.Valid {
		return false, nil
	}
	stale, err := db.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       row.ID,
		RunnerID:     row.RunnerID.UUID,
		StaleSeconds: int32(staleAfter.Seconds()),
	})
	if err != nil {
		return false, xerrors.Errorf("check chat heartbeat: %w", err)
	}
	return !stale, nil
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
