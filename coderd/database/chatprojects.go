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
// Locking every root before any sub-chat conflicts with code that locks
// chats in id order, such as SyncAgentChatsContextMCPResources, whatever
// order the roots lock in. Postgres then aborts one side with a deadlock
// error, which fails the delete; the caller may retry it.
func InChatProjectDeleteTx(ctx context.Context, db Store, projectID uuid.UUID, fn func(tx Store, rootIDs, chatIDs []uuid.UUID) error) error {
	return db.InTx(func(tx Store) error {
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
}
