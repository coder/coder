package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// This file owns the deterministic lock-race harness. On this branch its
// consumer is the per-user cap tests in user_caps_test.go; the stacked
// soft-delete-guard (#28546) and agent-memory (#28423) changes add more
// consumers, which is why it lives in its own file.

// stmt pairs a SQL statement with its bound arguments for runLockRace.
type stmt struct {
	sql  string
	args []any
}

// waitForBackendBlocked polls pg_stat_activity until the backend identified by
// pid is waiting on a heavyweight lock, proving the racing statement is
// blocked on a row lock rather than still executing.
func waitForBackendBlocked(ctx context.Context, t *testing.T, sqlDB *sql.DB, pid int) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var lockWaits int
		err := sqlDB.QueryRowContext(ctx, `
			SELECT count(*)
			FROM pg_stat_activity
			WHERE pid = $1 AND wait_event_type = 'Lock'
		`, pid).Scan(&lockWaits)
		return err == nil && lockWaits == 1
	}, testutil.IntervalFast, "wait for the backend to block on a row lock")
	require.NoError(t, ctx.Err(), "waiting for the blocked backend")
}

// runLockRace executes the blocking statements in one transaction, launches
// racing in its own transaction on a dedicated connection, deterministically
// waits for it to block on a lock held by the blocking transaction, executes
// beforeCommit inside the blocking transaction, commits it, commits the
// racing transaction when its statement succeeded, and returns the racing
// side's error. Both transactions run at the default isolation level.
func runLockRace(ctx context.Context, t *testing.T, sqlDB *sql.DB, blocking []stmt, racing stmt, beforeCommit []stmt) error {
	t.Helper()

	blockTx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	committed := false
	t.Cleanup(func() {
		if !committed {
			_ = blockTx.Rollback()
		}
	})
	for _, s := range blocking {
		_, err := blockTx.ExecContext(ctx, s.sql, s.args...)
		require.NoError(t, err)
	}

	raceConn, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = raceConn.Close() })

	var racePID int
	require.NoError(t, raceConn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&racePID))

	raceTx, err := raceConn.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = raceTx.Rollback() })

	raceResult := make(chan error, 1)
	go func() {
		_, err := raceTx.ExecContext(ctx, racing.sql, racing.args...)
		if err == nil {
			err = raceTx.Commit()
		}
		raceResult <- err
	}()

	waitForBackendBlocked(ctx, t, sqlDB, racePID)

	for _, s := range beforeCommit {
		_, err := blockTx.ExecContext(ctx, s.sql, s.args...)
		require.NoError(t, err)
	}
	require.NoError(t, blockTx.Commit())
	committed = true

	select {
	case err := <-raceResult:
		return err
	case <-ctx.Done():
		t.Fatalf("racing statement did not finish: %v", ctx.Err())
		return nil
	}
}
