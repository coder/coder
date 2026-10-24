package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// aibridge_interceptions.metadata is nullable and every insert path coalesces
// it, so a NULL only arrives from a migration, an import, or manual SQL. The
// session list projects it into json.RawMessage, which database/sql cannot
// convert a NULL into, so one such row used to fail the whole list.
func TestListAIBridgeSessionsNullMetadata(t *testing.T) {
	t.Parallel()

	setNullMetadata := func(ctx context.Context, t *testing.T, sqlDB *sql.DB, id uuid.UUID) {
		t.Helper()
		_, err := sqlDB.ExecContext(ctx, "UPDATE aibridge_interceptions SET metadata = NULL WHERE id = $1", id)
		require.NoError(t, err)
	}

	t.Run("OnlyInterception", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		ctx := context.Background()

		user := dbgen.User(t, db, database.User{})
		endedAt := dbtime.Now()
		interception := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID: user.ID,
			StartedAt:   endedAt.Add(-time.Minute),
		}, &endedAt)
		setNullMetadata(ctx, t, sqlDB, interception.ID)

		sessions, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{})
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		require.JSONEq(t, "{}", string(sessions[0].Metadata))
	})

	// The session's metadata comes from its first interception, so a NULL there
	// stays empty rather than falling through to a later interception's value.
	t.Run("FirstInterception", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		ctx := context.Background()

		user := dbgen.User(t, db, database.User{})
		sessionID := sql.NullString{String: "session-null-metadata", Valid: true}
		startedAt := dbtime.Now().Add(-time.Hour)

		firstEndedAt := startedAt.Add(time.Minute)
		first := dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     user.ID,
			StartedAt:       startedAt,
			ClientSessionID: sessionID,
		}, &firstEndedAt)
		secondEndedAt := startedAt.Add(3 * time.Minute)
		dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     user.ID,
			StartedAt:       startedAt.Add(2 * time.Minute),
			ClientSessionID: sessionID,
			Metadata:        json.RawMessage(`{"editor":"vscode"}`),
		}, &secondEndedAt)
		setNullMetadata(ctx, t, sqlDB, first.ID)

		sessions, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{})
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		require.JSONEq(t, "{}", string(sessions[0].Metadata))
	})
}
