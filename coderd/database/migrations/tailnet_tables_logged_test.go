package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

func TestEnsureTailnetTablesLogged(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	sqlDB := testSQLDB(t)
	require.NoError(t, migrations.Up(sqlDB))
	ctx := testutil.Context(t, testutil.WaitLong)

	// Children first, the reverse of the conversion back.
	for _, stmt := range []string{
		`ALTER TABLE tailnet_tunnels SET UNLOGGED`,
		`ALTER TABLE tailnet_peers SET UNLOGGED`,
		`ALTER TABLE tailnet_coordinators SET UNLOGGED`,
	} {
		_, err := sqlDB.ExecContext(ctx, stmt)
		require.NoError(t, err)
	}
	require.Equal(t, 3, countUnloggedTailnetTables(ctx, t, sqlDB))

	changed, err := migrations.EnsureTailnetTablesLogged(ctx, sqlDB)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 0, countUnloggedTailnetTables(ctx, t, sqlDB))

	changed, err = migrations.EnsureTailnetTablesLogged(ctx, sqlDB)
	require.NoError(t, err)
	require.False(t, changed, "already LOGGED tables must not be converted again")
}

func countUnloggedTailnetTables(ctx context.Context, t *testing.T, sqlDB *sql.DB) int {
	t.Helper()
	var unlogged int
	err := sqlDB.QueryRowContext(ctx, `
		SELECT count(*)
		FROM pg_class
		WHERE relname IN ('tailnet_coordinators', 'tailnet_peers', 'tailnet_tunnels')
		AND relpersistence = 'u'`).Scan(&unlogged)
	require.NoError(t, err)
	return unlogged
}
