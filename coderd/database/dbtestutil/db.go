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

// WillUsePostgres returns true if a call to NewDB() will return a real, postgres-backed Store and Pubsub.
func WillUsePostgres() bool {
	return os.Getenv("DB") != ""
}

// NewDB returns a new database.Store and pubsub.Pubsub.
// If the DB environment variable is set, a real postgres-backed
// Store and Pubsub will be returned.
// Additionally, if CODER_PG_CONNECTION_URL is set, it will be used
// to connect to the database.
// Otherwise, a new fake in-memory Store and Pubsub will be returned.
func NewDB(t testing.TB, seedFunc ...func(*sql.DB) error) (database.Store, pubsub.Pubsub) {
	t.Helper()

	db := dbfake.New()
	ps := pubsub.NewInMemory()
	if WillUsePostgres() {
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

		for i, f := range seedFunc {
			require.NoError(t, f(sqlDB), "database seed function %d failed", i+1)
		}

		ps, err = pubsub.New(context.Background(), sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ps.Close()
		})
	} else if len(seedFunc) > 0 {
		t.Fatal("cannot seed fake database, skip this test if not using postgres")
	}

	return db, ps
}
