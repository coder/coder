package database

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// LockChatProjectForDelete locks a project row, then its root chats, then
// those roots' sub-chats, and returns the root IDs and all locked chat
// IDs. tx must be a READ COMMITTED transaction, the InTx default. Each
// lock is its own statement because each statement takes a new snapshot
// there, and only a later snapshot sees rows whose inserts an earlier lock
// waited for: the project lock waits for root chats joining the project,
// and the root locks wait for sub-chats being created under them. Merging
// the statements, or running under REPEATABLE READ, would leave such chats
// out.
func LockChatProjectForDelete(ctx context.Context, tx Store, projectID uuid.UUID) (rootIDs, chatIDs []uuid.UUID, err error) {
	if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
		return nil, nil, xerrors.Errorf("lock project: %w", err)
	}
	rootIDs, err = tx.LockChatProjectRootChatsForDelete(ctx, projectID)
	if err != nil {
		return nil, nil, xerrors.Errorf("lock project root chats: %w", err)
	}
	if len(rootIDs) == 0 {
		return nil, nil, nil
	}
	subIDs, err := tx.LockSubChatsByRootIDsForDelete(ctx, rootIDs)
	if err != nil {
		return nil, nil, xerrors.Errorf("lock project sub-chats: %w", err)
	}
	return rootIDs, slices.Concat(rootIDs, subIDs), nil
}
