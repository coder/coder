package database

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// LockedChatProjectChat is a chat locked for deletion with its project.
// Callers use WorkerID and RunnerID to tell whether a worker holds it.
type LockedChatProjectChat struct {
	ID       uuid.UUID
	WorkerID uuid.NullUUID
	RunnerID uuid.NullUUID
	IsRoot   bool
}

// LockChatProjectDeleteBatch locks a project row, then up to limit of its
// root chats, then those roots' sub-chats. tx must be a READ COMMITTED
// transaction, the InTx default. Each lock is its own statement because
// each statement takes a new snapshot there, and only a later snapshot
// sees rows whose inserts an earlier lock waited for: the project lock
// waits for root chats joining the project, and the root locks wait for
// sub-chats being created under them. Merging the statements, or running
// under REPEATABLE READ, would leave such chats out of the batch.
func LockChatProjectDeleteBatch(ctx context.Context, tx Store, projectID uuid.UUID, limit int32) ([]LockedChatProjectChat, error) {
	if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
		return nil, xerrors.Errorf("lock project: %w", err)
	}
	roots, err := tx.LockChatProjectRootChatsForDelete(ctx, LockChatProjectRootChatsForDeleteParams{
		ProjectID:  projectID,
		LimitCount: limit,
	})
	if err != nil {
		return nil, xerrors.Errorf("lock project root chats: %w", err)
	}
	if len(roots) == 0 {
		return nil, nil
	}
	locked := make([]LockedChatProjectChat, 0, len(roots))
	rootIDs := make([]uuid.UUID, 0, len(roots))
	for _, row := range roots {
		locked = append(locked, LockedChatProjectChat{ID: row.ID, WorkerID: row.WorkerID, RunnerID: row.RunnerID, IsRoot: true})
		rootIDs = append(rootIDs, row.ID)
	}
	subs, err := tx.LockSubChatsByRootIDsForDelete(ctx, rootIDs)
	if err != nil {
		return nil, xerrors.Errorf("lock project sub-chats: %w", err)
	}
	for _, row := range subs {
		locked = append(locked, LockedChatProjectChat{ID: row.ID, WorkerID: row.WorkerID, RunnerID: row.RunnerID})
	}
	return locked, nil
}
