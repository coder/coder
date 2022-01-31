//go:build linux

package database_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/database"
	"github.com/coder/coder/database/postgres"
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
		connection, closeFn, err := postgres.Open()
		require.NoError(t, err)
		defer closeFn()
		db, err := sql.Open("postgres", connection)
		require.NoError(t, err)
		defer db.Close()
		err = database.Migrate(db)
		require.NoError(t, err)
	})

	t.Run("Twice", func(t *testing.T) {
		t.Parallel()
		connection, closeFn, err := postgres.Open()
		require.NoError(t, err)
		defer closeFn()
		db, err := sql.Open("postgres", connection)
		require.NoError(t, err)
		defer db.Close()
		err = database.Migrate(db)
		require.NoError(t, err)
		err = database.Migrate(db)
		require.NoError(t, err)
	})
}
