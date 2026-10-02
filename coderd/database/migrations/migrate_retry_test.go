package migrations_test

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

// TestUpWithFSRetriesDeadlocks checks that UpWithFS runs the migrations
// again when PostgreSQL aborts them as a deadlock victim, and only then.
// The migration counts its runs with a sequence, because nextval is not
// rolled back with the failed migration transaction.
func TestUpWithFSRetriesDeadlocks(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// failUntil makes the run with this sequence value and every
		// earlier run fail with errCode.
		failUntil   int
		errCode     string
		wantErr     string
		wantRuns    int
		wantRetries int
	}{
		{
			name:        "DeadlockOnce",
			failUntil:   1,
			errCode:     "deadlock_detected",
			wantRuns:    2,
			wantRetries: 1,
		},
		{
			name:      "OtherError",
			failUntil: 1,
			errCode:   "lock_not_available",
			wantErr:   "injected failure",
			wantRuns:  1,
		},
		{
			name:        "DeadlockEveryTime",
			failUntil:   100,
			errCode:     "deadlock_detected",
			wantErr:     "on all 5 attempts",
			wantRuns:    5,
			wantRetries: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			db := testSQLDB(t)
			_, err := db.ExecContext(ctx, `CREATE SEQUENCE migration_runs`)
			require.NoError(t, err)

			migs := fstest.MapFS{
				"000001_fail.up.sql": {Data: []byte(fmt.Sprintf(`
DO $$
BEGIN
	IF nextval('migration_runs') <= %d THEN
		RAISE EXCEPTION 'injected failure' USING ERRCODE = '%s';
	END IF;
END;
$$;`, tc.failUntil, tc.errCode))},
				"000001_fail.down.sql": {Data: []byte(`SELECT 1;`)},
			}
			sink := testutil.NewFakeSink(t)
			err = migrations.UpWithFS(db, migs, migrations.WithLogger(slog.Make(sink)))
			if tc.wantErr == "" {
				require.NoError(t, err)
				var version int
				var dirty bool
				require.NoError(t, db.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty))
				require.Equal(t, 1, version)
				require.False(t, dirty)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}

			var runs int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT last_value FROM migration_runs`).Scan(&runs))
			require.Equal(t, tc.wantRuns, runs, "migration runs")
			retries := sink.Entries(func(e slog.SinkEntry) bool { return e.Level == slog.LevelWarn })
			require.Len(t, retries, tc.wantRetries, "retry log lines")
			// Every failed run must end its transaction, or the retry
			// would wait for the advisory lock it holds.
			var openTxs int
			require.NoError(t, db.QueryRowContext(ctx, `
				SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND pid <> pg_backend_pid() AND xact_start IS NOT NULL`).Scan(&openTxs))
			require.Zero(t, openTxs, "open transactions left by UpWithFS")
		})
	}
}
