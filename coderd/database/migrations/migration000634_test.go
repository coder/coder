package migrations_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestMigration000634SessionAppMethod(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 633)

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	up, err := os.ReadFile("000634_session_app_method.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("000634_session_app_method.down.sql")
	require.NoError(t, err)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_usage_stats (start_time, end_time, template_id, user_id, usage_mins)
		VALUES (date_trunc('hour', now()), date_trunc('hour', now()) + interval '30 minutes',
			'22222222-2222-2222-2222-222222222222', '11111111-1111-1111-1111-111111111111', 30);
		INSERT INTO template_usage_stats_session_apps (start_time, template_id, user_id, app_name, usage_mins)
		SELECT start_time, template_id, user_id, app_name, mins
		FROM template_usage_stats, (VALUES ('vscode', 20), ('reconnecting_pty', 3), ('port_forwarding', 4)) AS apps(app_name, mins);
		INSERT INTO workspace_agent_stats (id, created_at, user_id, agent_id, workspace_id, template_id, session_counts)
		VALUES
			(gen_random_uuid(), now(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(),
				'{"vscode": 2, "ssh": 1, "reconnecting_pty": 1}'),
			(gen_random_uuid(), now() - interval '1 minute', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), '{}'),
			-- Older than the window the rollup and purge still read.
			(gen_random_uuid(), now() - interval '3 days', gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(),
				'{"vscode": 5}');
	`)
	require.NoError(t, err)

	queryStrings := func(t *testing.T, query string, args ...any) []string {
		t.Helper()
		rows, err := tx.QueryContext(ctx, query, args...)
		require.NoError(t, err)
		defer rows.Close()
		var got []string
		for rows.Next() {
			var s string
			require.NoError(t, rows.Scan(&s))
			got = append(got, s)
		}
		require.NoError(t, rows.Err())
		return got
	}
	sessionCounts := func(t *testing.T) []string {
		t.Helper()
		return queryStrings(t, `SELECT session_counts FROM workspace_agent_stats ORDER BY created_at DESC`)
	}

	_, err = tx.ExecContext(ctx, string(up))
	require.NoError(t, err)

	require.Equal(t, []string{
		"port_forwarding/port_forwarding=4",
		"reconnecting_pty/reconnecting_pty=3",
		"ssh/vscode=20",
	}, queryStrings(t, `SELECT method::text || '/' || app_name || '=' || usage_mins FROM template_usage_stats_session_apps ORDER BY 1`))

	counts := sessionCounts(t)
	require.Len(t, counts, 3)
	require.JSONEq(t, `{"ssh": {"vscode": 2, "ssh": 1}, "reconnecting_pty": {"reconnecting_pty": 1}}`, counts[0])
	require.JSONEq(t, `{}`, counts[1])
	require.JSONEq(t, `{"vscode": 5}`, counts[2])

	// The rollup's expansion keeps its row estimate; an inlined body would
	// estimate 10,000 rows per call.
	require.Equal(t, []string{"reconnecting_pty/reconnecting_pty", "ssh/ssh", "ssh/vscode"},
		queryStrings(t, `SELECT method::text || '/' || app_name FROM session_count_apps($1::jsonb) ORDER BY 1`, counts[0]))
	var plan string
	require.NoError(t, tx.QueryRowContext(ctx, `EXPLAIN SELECT * FROM session_count_apps('{}')`).Scan(&plan))
	require.Contains(t, plan, "rows=4 ")

	// A previous release's flat write is rejected.
	_, err = tx.ExecContext(ctx, `SAVEPOINT before_flat`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO workspace_agent_stats (id, created_at, user_id, agent_id, workspace_id, template_id, session_counts)
		VALUES (gen_random_uuid(), now(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), gen_random_uuid(), '{"vscode": 1}')
	`)
	require.ErrorContains(t, err, "workspace_agent_stats_session_counts_check")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT before_flat`)
	require.NoError(t, err)

	// The same app over another method gets its own row, and only session
	// methods are allowed.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_usage_stats_session_apps (start_time, template_id, user_id, method, app_name, usage_mins)
		SELECT start_time, template_id, user_id, 'port_forwarding', 'vscode', 15 FROM template_usage_stats
	`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SAVEPOINT before_tunnel`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO template_usage_stats_session_apps (start_time, template_id, user_id, method, app_name, usage_mins)
		SELECT start_time, template_id, user_id, 'tunnel', 'vscode', 1 FROM template_usage_stats
	`)
	require.ErrorContains(t, err, "template_usage_stats_session_apps_method_check")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT before_tunnel`)
	require.NoError(t, err)

	// Down merges methods per app, capped at the bucket width.
	_, err = tx.ExecContext(ctx, string(down))
	require.NoError(t, err)
	require.Equal(t, []sessionAppRow{{"port_forwarding", 4}, {"reconnecting_pty", 3}, {"vscode", 30}}, sessionRows(t, tx))
	counts = sessionCounts(t)
	require.JSONEq(t, `{"vscode": 2, "ssh": 1, "reconnecting_pty": 1}`, counts[0])
	require.JSONEq(t, `{}`, counts[1])
	require.JSONEq(t, `{"vscode": 5}`, counts[2])
	var fnExists bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT to_regprocedure('session_count_apps(jsonb)') IS NOT NULL`).Scan(&fnExists))
	require.False(t, fnExists)
}
