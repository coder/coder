//go:build linux

package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/database/postgres"

	_ "github.com/lib/pq"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPostgres(t *testing.T) {
	t.Parallel()

	connect, close, err := postgres.Open()
	require.NoError(t, err)
	defer close()
	db, err := sql.Open("postgres", connect)
	require.NoError(t, err)
	err = db.Ping()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}
