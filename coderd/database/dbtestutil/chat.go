package dbtestutil

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// InChatTransition runs fn inside a transaction that first locks the chat
// row and allocates a new snapshot_version, as every chat state transition
// does. The chat_messages triggers refuse a history write whose transaction
// has not written the chat row, so tests that seed or edit history without
// going through chatstate wrap the write in this.
func InChatTransition(ctx context.Context, db database.Store, chatID uuid.UUID, fn func(tx database.Store) error) error {
	return db.InTx(func(tx database.Store) error {
		if _, err := tx.LockChatAndBumpSnapshotVersion(ctx, chatID); err != nil {
			return xerrors.Errorf("lock chat and bump snapshot version: %w", err)
		}
		return fn(tx)
	}, nil)
}

// ExecChatHistorySQL runs one statement that writes chat_messages rows of
// chatID, inside a transaction that first bumps the chat's snapshot_version.
// See InChatTransition for why.
func ExecChatHistorySQL(ctx context.Context, sqlDB *sql.DB, chatID uuid.UUID, query string, args ...any) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE chats SET snapshot_version = snapshot_version + 1 WHERE id = $1`, chatID); err != nil {
		return xerrors.Errorf("bump chat snapshot version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	return tx.Commit()
}
