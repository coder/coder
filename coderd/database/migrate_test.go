//go:build linux

package database_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/postgres"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip()
		return
	}

	t.Run("Once", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := database.MigrateUp(db)
		require.NoError(t, err)
	})

	t.Run("Twice", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := database.MigrateUp(db)
		require.NoError(t, err)

		err = database.MigrateUp(db)
		require.NoError(t, err)
	})

	t.Run("UpDownUp", func(t *testing.T) {
		t.Parallel()

		db := testSQLDB(t)

		err := database.MigrateUp(db)
		require.NoError(t, err)

		err = database.MigrateDown(db)
		require.NoError(t, err)

		err = database.MigrateUp(db)
		require.NoError(t, err)
	})
}

func testSQLDB(t testing.TB) *sql.DB {
	t.Helper()

	connection, closeFn, err := postgres.Open()
	require.NoError(t, err)
	t.Cleanup(closeFn)

	db, err := sql.Open("postgres", connection)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
