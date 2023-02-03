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
)

func NewDB(t *testing.T) (database.Store, database.Pubsub) {
	t.Helper()

	db := dbfake.New()
	pubsub := database.NewPubsubInMemory()
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

		pubsub, err = database.NewPubsub(context.Background(), sqlDB, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = pubsub.Close()
		})
	}

	return db, pubsub
}
