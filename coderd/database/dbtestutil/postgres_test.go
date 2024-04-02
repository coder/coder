//go:build linux

package dbtestutil_test

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// nolint:paralleltest
func TestPostgres(t *testing.T) {
	// postgres.Open() seems to be creating race conditions when run in parallel.
	// t.Parallel()

	if testing.Short() {
		t.SkipNow()
		return
	}

	connect, closePg, err := dbtestutil.Open()
	require.NoError(t, err)
	defer closePg()
	db, err := sql.Open("postgres", connect)
	require.NoError(t, err)
	err = db.Ping()
	require.NoError(t, err)
	err = db.Close()
	require.NoError(t, err)
}
