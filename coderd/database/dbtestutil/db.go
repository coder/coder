package dbtestutil

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/coderd/database/pubsub"
)

func NewDB(t testing.TB) (database.Store, pubsub.Pubsub) {
	t.Helper()

	db := dbfake.New()
	ps := pubsub.NewInMemory()
	if os.Getenv("DB") != "" {
		connectionURL := os.Getenv("CODER_PG_CONNECTION_URL")
		if connectionURL == "" {
			var (
				err     error
				closePg func()
			)
			connectionURL, closePg, err = postgres.Open()
			require.NoError(t, err)
			t.Cleanup(closePg)
		}
		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
		db = database.New(sqlDB)

		ps, err = pubsub.New(context.Background(), sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ps.Close()
		})
	}

	return db, ps
}
