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

// DeleteChatProject deletes a project and publishes hard_deleted watch
// events for the chats it deleted. See DeleteChatProjectWithoutEvents.
func (p *Server) DeleteChatProject(ctx context.Context, projectID uuid.UUID) ([]database.Chat, error) {
	deleted, err := DeleteChatProjectWithoutEvents(ctx, p.db, projectID, p.inFlightChatStaleAfter)
	p.publishChatPubsubEvents(deleted, codersdk.ChatWatchEventKindHardDeleted)
	return deleted, err
}

// DeleteChatProjectWithoutEvents deletes a project together with its root
// chats, every user's, and their sub-chats, and returns the chats it
// deleted. It fails with ErrChatProjectHasRunningChats while a live worker
// holds any of them, so in-flight turns are never cut off; a lease whose
// heartbeat is older than staleAfter does not count. Chats go in batches,
// so a failure can leave the project with some chats already deleted, and
// the returned chats are those; retrying finishes the job. Callers must
// have authorized deleting the project.
func DeleteChatProjectWithoutEvents(ctx context.Context, db database.Store, projectID uuid.UUID, staleAfter time.Duration) ([]database.Chat, error) {
	//nolint:gocritic // Sharees own some of the chats; the caller authorized deleting the project.
	chatdCtx := dbauthz.AsChatd(ctx)
	var deleted []database.Chat
	for {
		var (
			done  bool
			batch []database.Chat
		)
		err := db.InTx(func(tx database.Store) error {
			batch = nil
			// Locking the project blocks new root chats from joining it
			// until this batch commits.
			if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
				return xerrors.Errorf("lock project: %w", err)
			}
			roots, err := tx.LockChatProjectRootChatsForDelete(chatdCtx, database.LockChatProjectRootChatsForDeleteParams{
				ProjectID:  projectID,
				LimitCount: chatProjectDeleteBatchSize,
			})
			if err != nil {
				return xerrors.Errorf("lock project chats: %w", err)
			}
			if len(roots) == 0 {
				if err := tx.DeleteChatProjectByID(ctx, projectID); err != nil {
					return xerrors.Errorf("delete project: %w", err)
				}
				done = true
				return nil
			}
			locked := make([]lockedChat, 0, len(roots))
			rootIDs := make([]uuid.UUID, 0, len(roots))
			for _, row := range roots {
				locked = append(locked, lockedChat{ID: row.ID, WorkerID: row.WorkerID, RunnerID: row.RunnerID})
				rootIDs = append(rootIDs, row.ID)
			}
			subs, err := tx.LockSubChatsByRootIDsForDelete(chatdCtx, rootIDs)
			if err != nil {
				return xerrors.Errorf("lock project sub-chats: %w", err)
			}
			for _, row := range subs {
				locked = append(locked, lockedChat{ID: row.ID, WorkerID: row.WorkerID, RunnerID: row.RunnerID})
			}
			ids := make([]uuid.UUID, 0, len(locked))
			for _, chat := range locked {
				running, err := chatHeldByLiveWorker(chatdCtx, tx, chat, staleAfter)
				if err != nil {
					return err
				}
				if running {
					return ErrChatProjectHasRunningChats
				}
				ids = append(ids, chat.ID)
			}
			chats, err := tx.GetChatsByIDs(chatdCtx, ids)
			if err != nil {
				return xerrors.Errorf("get project chats: %w", err)
			}
			if err := tx.DeleteChatsByIDs(chatdCtx, ids); err != nil {
				return xerrors.Errorf("delete project chats: %w", err)
			}
			batch = chats
			return nil
		}, nil)
		if err != nil {
			return deleted, err
		}
		deleted = append(deleted, batch...)
		if done {
			return deleted, nil
		}
	}
}

type lockedChat struct {
	ID       uuid.UUID
	WorkerID uuid.NullUUID
	RunnerID uuid.NullUUID
}

// chatHeldByLiveWorker reports whether a worker with a fresh heartbeat holds
// the chat. A turn can end with worker_id still set, and a replica that dies
// then never clears it, so worker_id alone would block deletion forever.
func chatHeldByLiveWorker(ctx context.Context, db database.Store, chat lockedChat, staleAfter time.Duration) (bool, error) {
	if !chat.WorkerID.Valid || !chat.RunnerID.Valid {
		return false, nil
	}
	stale, err := db.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID:       chat.ID,
		RunnerID:     chat.RunnerID.UUID,
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
