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
// Two lock orders can deadlock with the delete. Locking every root before
// any sub-chat conflicts with code that locks chats in id order, such as
// SyncAgentChatsContextMCPResources, whatever order the roots lock in. The
// heartbeat cascade below conflicts with lease renewal, which locks
// heartbeat rows in its own order.
//
// Postgres then aborts one of the two transactions. If it aborts the
// delete, the returned error matches IsDeadlockError, the transaction
// rolls back with fn's writes, and a caller that retries must retry the
// whole transaction, including any outer one db belongs to. If it aborts
// the other transaction, that transaction fails instead.
//
// DeleteChatFamiliesByRootIDs cascades to the chats' heartbeat rows and
// holds their row locks until the outermost transaction commits. A replica
// renewing one of those leases holds the deployment-wide capacity
// admission lock while it waits, up to HeartbeatLockTimeout, and then
// renews none of its leases that tick. In fn, call
// DeleteChatMessagesByChatIDs before DeleteChatFamiliesByRootIDs, and keep
// the work after the chat delete to the project row.
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
