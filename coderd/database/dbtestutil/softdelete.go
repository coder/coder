package dbtestutil

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// SoftDeleteUserKeepingRows marks the user deleted while suppressing the
// delete_deleted_user_resources cleanup trigger, so the user's child rows
// (api_keys, user_links, and the other guarded tables) survive. This
// reconstructs the orphaned-row state that could exist before migration
// 000591 closed the insert-vs-soft-delete race (the insert guards now also
// reject new rows for deleted users, so the state can only be constructed
// this way). Tests use it to prove such legacy rows stay inert.
func SoftDeleteUserKeepingRows(ctx context.Context, t testing.TB, sqlDB *sql.DB, userID uuid.UUID) {
	t.Helper()
	// One transaction: transactional DDL keeps the disabled trigger
	// invisible to concurrent sessions (which may share this database under
	// CODER_PG_CONNECTION_URL) and rolls the disable back on failure.
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.ExecContext(ctx, `ALTER TABLE users DISABLE TRIGGER trigger_update_users`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE users SET deleted = true WHERE id = $1`, userID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `ALTER TABLE users ENABLE TRIGGER trigger_update_users`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	committed = true
}
