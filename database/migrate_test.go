//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/coder/coder/database"
	"github.com/coder/coder/database/postgres"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMigrate(t *testing.T) {
	t.Parallel()

	connection, closeFn, err := postgres.Open()
	require.NoError(t, err)
	defer closeFn()
	db, err := sql.Open("postgres", connection)
	require.NoError(t, err)
	err = database.Migrate(context.Background(), "postgres", db)
	require.NoError(t, err)
}
