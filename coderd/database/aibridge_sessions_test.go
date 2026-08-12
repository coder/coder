package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

// TestAIBridgeSessionTriggers covers the triggers that keep aibridge_sessions in
// sync with aibridge_interceptions and aibridge_user_prompts. The table is only
// reachable through ListAIBridgeSessions, so assertions go through it: the
// timestamps are read from the returned rows, and the denormalized filter
// columns are read by filtering on them.
func TestAIBridgeSessionTriggers(t *testing.T) {
	t.Parallel()

	// sessionByID returns the single session with the given ID, or nil.
	sessionByID := func(t *testing.T, db database.Store, sessionID string) *database.ListAIBridgeSessionsRow {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		//nolint:exhaustruct // Only the session_id filter is relevant.
		rows, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
			SessionID: sessionID,
			Limit:     10,
		})
		require.NoError(t, err)
		if len(rows) == 0 {
			return nil
		}
		require.Len(t, rows, 1)
		return &rows[0]
	}

	// interception inserts an interception, optionally completing it.
	interception := func(t *testing.T, db database.Store, userID uuid.UUID, sessionID, model string, startedAt time.Time, endedAt *time.Time) database.AIBridgeInterception {
		t.Helper()
		//nolint:exhaustruct // dbgen fills the rest.
		return dbgen.AIBridgeInterception(t, db, database.InsertAIBridgeInterceptionParams{
			InitiatorID:     userID,
			Provider:        "anthropic",
			ProviderName:    "anthropic-prod",
			Model:           model,
			Client:          sql.NullString{String: "claude-code", Valid: true},
			ClientSessionID: sql.NullString{String: sessionID, Valid: true},
			StartedAt:       startedAt,
		}, endedAt)
	}

	t.Run("InflightInterceptionIsNotTracked", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		start := dbtestutil.NowInDefaultTimezone()

		interception(t, db, user.ID, "inflight", "opus", start, nil)

		require.Nil(t, sessionByID(t, db, "inflight"),
			"a session with no completed interception must not appear")
	})

	// The prompt trigger cannot update a row that does not exist yet, and
	// prompts are always recorded before the interception ends. The interception
	// trigger has to pick them up when it creates the row.
	t.Run("PromptRecordedBeforeInterceptionEnded", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		start := dbtestutil.NowInDefaultTimezone()
		promptAt := start.Add(5 * time.Minute)
		endedAt := start.Add(6 * time.Minute)

		ctx := testutil.Context(t, testutil.WaitLong)

		intc := interception(t, db, user.ID, "prompt-first", "opus", start, nil)
		//nolint:exhaustruct // dbgen fills the rest.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: intc.ID,
			CreatedAt:      promptAt,
		})
		_, err := db.UpdateAIBridgeInterceptionEnded(ctx, database.UpdateAIBridgeInterceptionEndedParams{
			ID:      intc.ID,
			EndedAt: endedAt,
		})
		require.NoError(t, err)

		session := sessionByID(t, db, "prompt-first")
		require.NotNil(t, session)
		require.WithinDuration(t, promptAt, session.LastActiveAt, time.Second,
			"last_active_at must come from the prompt, not the interception start")
	})

	t.Run("PromptlessSessionFallsBackToStartedAt", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		start := dbtestutil.NowInDefaultTimezone()
		endedAt := start.Add(time.Minute)

		interception(t, db, user.ID, "no-prompts", "opus", start, &endedAt)

		session := sessionByID(t, db, "no-prompts")
		require.NotNil(t, session)
		require.WithinDuration(t, start, session.LastActiveAt, time.Second,
			"a session with no prompts must fall back to started_at")
	})

	t.Run("PromptAfterRowExistsUpdatesLastActiveAt", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		start := dbtestutil.NowInDefaultTimezone()
		endedAt := start.Add(time.Minute)

		// First interception creates the row.
		interception(t, db, user.ID, "live", "opus", start, &endedAt)

		// Second interception, then a prompt against it once the row exists.
		secondStart := start.Add(10 * time.Minute)
		secondEnded := secondStart.Add(time.Minute)
		second := interception(t, db, user.ID, "live", "opus", secondStart, &secondEnded)

		laterPrompt := start.Add(20 * time.Minute)
		//nolint:exhaustruct // dbgen fills the rest.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: second.ID,
			CreatedAt:      laterPrompt,
		})

		session := sessionByID(t, db, "live")
		require.NotNil(t, session)
		require.WithinDuration(t, laterPrompt, session.LastActiveAt, time.Second)

		// An older prompt must not move it backwards.
		//nolint:exhaustruct // dbgen fills the rest.
		dbgen.AIBridgeUserPrompt(t, db, database.InsertAIBridgeUserPromptParams{
			InterceptionID: second.ID,
			CreatedAt:      start.Add(2 * time.Minute),
		})

		session = sessionByID(t, db, "live")
		require.NotNil(t, session)
		require.WithinDuration(t, laterPrompt, session.LastActiveAt, time.Second,
			"an out-of-order prompt must not move last_active_at backwards")
	})

	// A second interception must widen the session rather than replace it, and
	// must do so regardless of the order the interceptions complete in.
	t.Run("SecondInterceptionMergesIntoTheSameSession", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		ctx := testutil.Context(t, testutil.WaitLong)
		later := dbtestutil.NowInDefaultTimezone()
		earlier := later.Add(-time.Hour)

		// Complete the later interception first, so started_at has to move back.
		laterEnded := later.Add(time.Minute)
		interception(t, db, user.ID, "merged", "opus", later, &laterEnded)
		earlierEnded := earlier.Add(time.Minute)
		interception(t, db, user.ID, "merged", "haiku", earlier, &earlierEnded)

		session := sessionByID(t, db, "merged")
		require.NotNil(t, session)
		require.EqualValues(t, 2, session.Threads)
		require.WithinDuration(t, later, session.LastActiveAt, time.Second,
			"last_active_at must keep the latest activity")

		// The denormalized arrays are only observable through the filters they
		// exist to serve, since the response builds its own from the page.
		for _, model := range []string{"opus", "haiku"} {
			//nolint:exhaustruct // Only the model filter is relevant.
			rows, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
				Model: model,
				Limit: 10,
			})
			require.NoError(t, err)
			require.Len(t, rows, 1, "filtering by model %q should find the session", model)
			require.Equal(t, "merged", rows[0].SessionID)
		}

		// started_at moved back to the earlier interception, so a window that
		// only contains the earlier one still matches.
		//nolint:exhaustruct // Only the time filter is relevant.
		rows, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
			StartedBefore: earlier.Add(time.Minute),
			Limit:         10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "merged", rows[0].SessionID)
	})

	t.Run("ClientIsFilterable", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{})
		ctx := testutil.Context(t, testutil.WaitLong)
		start := dbtestutil.NowInDefaultTimezone()
		endedAt := start.Add(time.Minute)

		interception(t, db, user.ID, "with-client", "opus", start, &endedAt)

		//nolint:exhaustruct // Only the client filter is relevant.
		rows, err := db.ListAIBridgeSessions(ctx, database.ListAIBridgeSessionsParams{
			Client: "claude-code",
			Limit:  10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "with-client", rows[0].SessionID)
	})
}
