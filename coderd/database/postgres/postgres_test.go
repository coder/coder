//go:build linux

package postgres_test

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/coderd/database/postgres"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPostgres(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip()
		return
	}

	connect, closePg, err := postgres.Open()
	require.NoError(t, err)
	defer closePg()
	db, err := sql.Open("postgres", connect)
	require.NoError(t, err)
	err = db.Ping()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}
