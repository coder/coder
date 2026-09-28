package migrations_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

// stepTo advances the migrations to version and fails if it is never reached.
func stepTo(t *testing.T, sqlDB *sql.DB, version uint) {
	t.Helper()

	next, err := migrations.Stepper(sqlDB)
	require.NoError(t, err)
	for {
		got, more, err := next()
		require.NoError(t, err)
		if !more {
			t.Fatalf("migration %d not found", version)
		}
		if got == version {
			return
		}
	}
}

// sessionAppRow is one row of template_usage_stats_session_apps.
type sessionAppRow struct {
	appName   string
	usageMins int64
}

// sessionRows reads the child table ordered by app name.
func sessionRows(t *testing.T, tx *sql.Tx) []sessionAppRow {
	t.Helper()

	rows, err := tx.Query(`SELECT app_name, usage_mins FROM template_usage_stats_session_apps ORDER BY app_name`)
	require.NoError(t, err)
	defer rows.Close()

	var got []sessionAppRow
	for rows.Next() {
		var row sessionAppRow
		require.NoError(t, rows.Scan(&row.appName, &row.usageMins))
		got = append(got, row)
	}
	require.NoError(t, rows.Err())
	return got
}

// TestMigration000596TemplateUsageStatsSessionUsage covers the conversion of
// the fixed per-family minute columns into the child table, which the
// testdata/fixtures run does not reach: its template_usage_stats rows record
// no session minutes, so the backfill matches zero rows in CI.
//
//nolint:tparallel,paralleltest // Subtests share one database with transaction-local fixtures.
func TestMigration000596TemplateUsageStatsSessionUsage(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 595)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	migrationSQL, err := os.ReadFile("000596_template_usage_stats_session_usage.up.sql")
	require.NoError(t, err)
	// insertUsageStats writes one row per minute set, keyed by
	// (ssh, sftp, reconnecting_pty, vscode, jetbrains).
	insertUsageStats := func(t *testing.T, tx *sql.Tx, mins ...[5]int) {
		t.Helper()

		for i, m := range mins {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO template_usage_stats (
					start_time, end_time, template_id, user_id, median_latency_ms,
					usage_mins, ssh_mins, sftp_mins, reconnecting_pty_mins,
					vscode_mins, jetbrains_mins, app_usage_mins
				) VALUES (
					date_trunc('hour', statement_timestamp()) + $1::bigint * interval '30 minutes',
					date_trunc('hour', statement_timestamp()) + $1::bigint * interval '30 minutes' + interval '30 minutes',
					gen_random_uuid(), gen_random_uuid(), NULL, 30, $2, $3, $4, $5, $6, NULL
				)
			`, i, m[0], m[1], m[2], m[3], m[4])
			require.NoError(t, err)
		}
	}

	t.Run("up", func(t *testing.T) {
		// The fixed columns only ever recorded the family, so the converted
		// rows carry a family name as their app name. The registry maps each
		// of those back to itself, so reads attribute them unchanged.
		tests := []struct {
			name     string
			mins     [][5]int
			wantApps []sessionAppRow
		}{
			{name: "no rows"},
			{
				name: "every family",
				mins: [][5]int{{1, 2, 3, 4, 5}},
				wantApps: []sessionAppRow{
					{"jetbrains", 5}, {"reconnecting_pty", 3}, {"sftp", 2}, {"ssh", 1}, {"vscode", 4},
				},
			},
			{
				// The rollup has never written sftp_mins, but a row that has
				// a value must not lose it.
				name:     "sftp only",
				mins:     [][5]int{{0, 7, 0, 0, 0}},
				wantApps: []sessionAppRow{{"sftp", 7}},
			},
			{
				name:     "zero minutes produce no row",
				mins:     [][5]int{{5, 0, 0, 6, 0}},
				wantApps: []sessionAppRow{{"ssh", 5}, {"vscode", 6}},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tx, err := sqlDB.BeginTx(ctx, nil)
				require.NoError(t, err)
				t.Cleanup(func() { _ = tx.Rollback() })

				insertUsageStats(t, tx, tt.mins...)

				_, err = tx.ExecContext(ctx, string(migrationSQL))
				require.NoError(t, err)

				require.Equal(t, tt.wantApps, sessionRows(t, tx))
			})
		}
	})

	// The child table only holds session usage of buckets that exist, and it
	// follows its bucket when that is deleted, which is how dbpurge and the
	// retention deletes stay correct without knowing about it.
	t.Run("child rows require and follow their bucket", func(t *testing.T) {
		tx, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		_, err = tx.ExecContext(ctx, string(migrationSQL))
		require.NoError(t, err)

		// The failed insert aborts the transaction, so take a savepoint the
		// rest of the subtest can carry on from.
		_, err = tx.ExecContext(ctx, `SAVEPOINT before_orphan`)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO template_usage_stats_session_apps VALUES (
				date_trunc('hour', statement_timestamp()),
				gen_random_uuid(), gen_random_uuid(), 'ssh', 1
			)
		`)
		require.ErrorContains(t, err, "violates foreign key constraint")
		_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT before_orphan`)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO template_usage_stats (
				start_time, end_time, template_id, user_id, median_latency_ms,
				usage_mins, app_usage_mins
			) VALUES (
				date_trunc('hour', statement_timestamp()),
				date_trunc('hour', statement_timestamp()) + interval '30 minutes',
				'22222222-2222-2222-2222-222222222222'::uuid,
				'11111111-1111-1111-1111-111111111111'::uuid,
				NULL, 30, NULL
			)
		`)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO template_usage_stats_session_apps (
				start_time, template_id, user_id, app_name, usage_mins
			) VALUES (
				date_trunc('hour', statement_timestamp()),
				'22222222-2222-2222-2222-222222222222'::uuid,
				'11111111-1111-1111-1111-111111111111'::uuid, 'zed', 1
			)
		`)
		require.NoError(t, err)

		_, err = tx.ExecContext(ctx, `DELETE FROM template_usage_stats`)
		require.NoError(t, err)
		require.Empty(t, sessionRows(t, tx))
	})
}

// TestMigration000596ChainFrom589 walks the whole window this change spans,
// 589 up to 596 and back down to 589, with data present at every step. The
// isolated 596 tests start at 595, so they never see 590 converting raw
// session counts the rollup has not consumed, which is the state an upgrade
// actually finds.
//
//nolint:tparallel,paralleltest // Subtests share one database with transaction-local fixtures.
func TestMigration000596ChainFrom589(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 589)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	up590, err := os.ReadFile("000590_workspace_agent_session_counts.up.sql")
	require.NoError(t, err)
	down590, err := os.ReadFile("000590_workspace_agent_session_counts.down.sql")
	require.NoError(t, err)
	up596, err := os.ReadFile("000596_template_usage_stats_session_usage.up.sql")
	require.NoError(t, err)
	down596, err := os.ReadFile("000596_template_usage_stats_session_usage.down.sql")
	require.NoError(t, err)

	// backlogHours spans more than a day, and two backlogged rows sit inside
	// that span, so 590's backlog warning branch runs. That is the slow path an
	// upgrade of a stalled deployment takes.
	const backlogHours = 48

	// chainStep names a migration file so a failure says which step broke.
	type chainStep struct {
		name string
		sql  []byte
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	// One rolled-up half hour, so 590 has a watermark to measure the backlog
	// against, and so 596 has a row whose fixed family minutes must convert.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_usage_stats (
			start_time, end_time, template_id, user_id, median_latency_ms,
			usage_mins, ssh_mins, sftp_mins, reconnecting_pty_mins,
			vscode_mins, jetbrains_mins, app_usage_mins
		) VALUES (
			date_trunc('hour', statement_timestamp()) - $1::bigint * interval '1 hour',
			date_trunc('hour', statement_timestamp()) - $1::bigint * interval '1 hour' + interval '30 minutes',
			'22222222-2222-2222-2222-222222222222'::uuid,
			'11111111-1111-1111-1111-111111111111'::uuid,
			NULL, 30, 3, 2, 0, 4, 0, NULL
		)
	`, backlogHours)
	require.NoError(t, err)

	// Raw agent stats the rollup has not consumed: two inside the backlog,
	// spanning more than a day so 590 measures a wide backlog and takes its
	// warning path, and one older than the window 590 converts, which is the
	// watermark less the day DeleteOldWorkspaceAgentStats retains. 590 leaves
	// that one as an empty map because template_usage_stats accounts for it.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO workspace_agent_stats (
			id, created_at, user_id, agent_id, workspace_id, template_id,
			connection_count, session_count_vscode, session_count_ssh
		) VALUES (
			gen_random_uuid(), statement_timestamp() - interval '5 minutes',
			'11111111-1111-1111-1111-111111111111'::uuid, gen_random_uuid(),
			gen_random_uuid(), '22222222-2222-2222-2222-222222222222'::uuid, 1, 2, 1
		), (
			gen_random_uuid(), statement_timestamp() - ($1::bigint - 1) * interval '1 hour',
			'11111111-1111-1111-1111-111111111111'::uuid, gen_random_uuid(),
			gen_random_uuid(), '22222222-2222-2222-2222-222222222222'::uuid, 1, 4, 2
		), (
			gen_random_uuid(), statement_timestamp() - $1::bigint * interval '1 hour' - interval '30 hours',
			'11111111-1111-1111-1111-111111111111'::uuid, gen_random_uuid(),
			gen_random_uuid(), '22222222-2222-2222-2222-222222222222'::uuid, 1, 5, 5
		)
	`, backlogHours)
	require.NoError(t, err)

	for _, step := range []chainStep{{"590 up", up590}, {"596 up", up596}} {
		_, err = tx.ExecContext(ctx, string(step.sql))
		require.NoError(t, err, "%s", step.name)
	}

	// 590 converted both backlogged rows under the canonical family names, and
	// zeroed the row the rollup had already consumed.
	rows, err := tx.QueryContext(ctx, `
		SELECT session_counts FROM workspace_agent_stats ORDER BY created_at
	`)
	require.NoError(t, err)
	var gotCounts []string
	for rows.Next() {
		var counts []byte
		require.NoError(t, rows.Scan(&counts))
		gotCounts = append(gotCounts, string(counts))
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, gotCounts, 3)
	require.JSONEq(t, `{}`, gotCounts[0], "older than the converted window")
	require.JSONEq(t, `{"vscode": 4, "ssh": 2}`, gotCounts[1], "backlogged, over a day old")
	require.JSONEq(t, `{"vscode": 2, "ssh": 1}`, gotCounts[2], "backlogged, recent")

	// 596 converted the fixed family minutes under their family names.
	require.Equal(t, []sessionAppRow{{"sftp", 2}, {"ssh", 3}, {"vscode", 4}},
		sessionRows(t, tx))

	// A real app name folds into its family column, and an unregistered one
	// counts as ssh rather than losing its minutes.
	_, err = tx.ExecContext(ctx, `
		UPDATE template_usage_stats_session_apps SET app_name = 'cursor' WHERE app_name = 'vscode'
	`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_usage_stats_session_apps (
			start_time, template_id, user_id, app_name, usage_mins
		)
		SELECT start_time, template_id, user_id, 'some_new_ide', 6
		FROM template_usage_stats
	`)
	require.NoError(t, err)

	// Back down: 596 restores the fixed columns, then 590 restores the fixed
	// session counts, landing on the 589 schema.
	for _, step := range []chainStep{{"596 down", down596}, {"590 down", down590}} {
		_, err = tx.ExecContext(ctx, string(step.sql))
		require.NoError(t, err, "%s", step.name)
	}

	var ssh, sftp, reconnectingPTY, vscode, jetbrains int64
	err = tx.QueryRowContext(ctx, `
		SELECT ssh_mins, sftp_mins, reconnecting_pty_mins, vscode_mins, jetbrains_mins
		FROM template_usage_stats
	`).Scan(&ssh, &sftp, &reconnectingPTY, &vscode, &jetbrains)
	require.NoError(t, err)
	require.EqualValues(t, 9, ssh, "ssh plus the unregistered app name")
	require.EqualValues(t, 2, sftp)
	require.EqualValues(t, 0, reconnectingPTY)
	require.EqualValues(t, 4, vscode)
	require.EqualValues(t, 0, jetbrains)

	var backloggedVSCode, backloggedSSH int64
	err = tx.QueryRowContext(ctx, `
		SELECT session_count_vscode, session_count_ssh
		FROM workspace_agent_stats
		ORDER BY created_at DESC
		LIMIT 1
	`).Scan(&backloggedVSCode, &backloggedSSH)
	require.NoError(t, err)
	require.EqualValues(t, 2, backloggedVSCode)
	require.EqualValues(t, 1, backloggedSSH)

	// Everything both migrations added is gone, so the 589 schema is back.
	var leftovers int
	err = tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM information_schema.columns
				WHERE table_name = 'workspace_agent_stats' AND column_name = 'session_counts')
			+ (SELECT COUNT(*) FROM information_schema.tables
				WHERE table_name = 'template_usage_stats_session_apps')
	`).Scan(&leftovers)
	require.NoError(t, err)
	require.Zero(t, leftovers)
}
