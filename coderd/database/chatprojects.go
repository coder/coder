package database

import (
	"context"
	"database/sql"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// InChatProjectDeleteTx locks a project row, then its root chats, then
// those roots' sub-chats, and calls fn in the same transaction with the
// root IDs and all locked chat IDs. Each lock is its own statement of a
// READ COMMITTED transaction: there each statement takes a new snapshot,
// and only a later snapshot sees rows whose inserts an earlier lock waited
// for. The project lock waits for root chats joining the project, and the
// root locks wait for sub-chats being created under them. Merging the
// statements, or running under REPEATABLE READ, would leave such chats out.
// When db is already a transaction, InTx reuses it and its isolation level,
// which must then be READ COMMITTED.
//
// Root chats lock in index-scan order, so this can deadlock with
// SyncAgentChatsContextMCPResources, which locks chats in id order.
// Postgres aborts one side, and the transaction is retried.
func InChatProjectDeleteTx(ctx context.Context, db Store, projectID uuid.UUID, fn func(tx Store, rootIDs, chatIDs []uuid.UUID) error) error {
	var err error
	for range maxRetries {
		err = db.InTx(func(tx Store) error {
			if _, err := tx.GetChatProjectByIDForUpdate(ctx, projectID); err != nil {
				return xerrors.Errorf("lock project: %w", err)
			}
			rootIDs, err := tx.LockChatProjectRootChatsForDelete(ctx, projectID)
			if err != nil {
				return xerrors.Errorf("lock project root chats: %w", err)
			}
			var subIDs []uuid.UUID
			if len(rootIDs) > 0 {
				subIDs, err = tx.LockSubChatsByRootIDsForDelete(ctx, rootIDs)
				if err != nil {
					return xerrors.Errorf("lock project sub-chats: %w", err)
				}
			}
			return fn(tx, rootIDs, slices.Concat(rootIDs, subIDs))
		}, &TxOptions{Isolation: sql.LevelReadCommitted, TxIdentifier: "delete_chat_project"})
		if !IsDeadlockError(err) {
			return err
		}
	}
	return err
}
