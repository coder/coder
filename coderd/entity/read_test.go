package entity_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

func TestLifecycleEntries(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsEntriesOldestFirst", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		subject := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()}
		for _, event := range []entity.Event{entity.EventCreated, "second", "third"} {
			_, err := entity.AppendEntry(ctx, db, entity.Entry{
				Event:   event,
				Subject: subject,
				Actor:   actor,
			})
			require.NoError(t, err)
		}

		bySubject, err := entity.LifecycleEntriesBySubject(ctx, testutil.Logger(t), db, subject)
		require.NoError(t, err)
		require.Len(t, bySubject, 3)
		require.Equal(t, []string{"created", "second", "third"},
			[]string{bySubject[0].Event, bySubject[1].Event, bySubject[2].Event},
			"entries should come back in the order they were written")

		byActor, err := entity.LifecycleEntriesByActor(ctx, testutil.Logger(t), db, actor)
		require.NoError(t, err)
		require.Len(t, byActor, 3)
	})

	// The limit is a backstop against a condition that should be impossible,
	// so the entries are written directly rather than through AppendEntry.
	// Producing them the ordinary way would take as long as the condition is
	// supposed to be unreachable.
	t.Run("FailsWhenAnEntityHasMoreEntriesThanItsLifecycleAllows", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		actor := uuid.New()
		fillJournal(ctx, t, sqlDB, actor, entity.LifecycleEntryLimit+1)

		// The read logs at error level on purpose, and slogtest fails a test
		// that logs at error level, also on purpose. This is the one place
		// that log is expected, so this is the one place it is allowed.
		log := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

		_, err := entity.LifecycleEntriesByActor(ctx, log, db, entity.Ref{
			Type: entity.TypeWorkspaceAgent,
			ID:   actor,
		})
		require.ErrorIs(t, err, entity.ErrTooManyEntries)
	})

	// One below the limit is an ordinary read. Without this, a limit that
	// rejected everything would pass the case above.
	t.Run("SucceedsAtTheLimit", func(t *testing.T) {
		t.Parallel()

		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		actor := uuid.New()
		fillJournal(ctx, t, sqlDB, actor, entity.LifecycleEntryLimit)

		entries, err := entity.LifecycleEntriesByActor(ctx, testutil.Logger(t), db, entity.Ref{
			Type: entity.TypeWorkspaceAgent,
			ID:   actor,
		})
		require.NoError(t, err)
		require.Len(t, entries, entity.LifecycleEntryLimit)
	})

	t.Run("RejectsAnUnknownType", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := entity.LifecycleEntriesByActor(ctx, testutil.Logger(t), db, entity.Ref{
			Type: "sandbox",
			ID:   uuid.New(),
		})
		require.ErrorContains(t, err, "names no kind of entity")

		_, err = entity.LifecycleEntriesBySubject(ctx, testutil.Logger(t), db, entity.Ref{
			Type: "sandbox",
			ID:   uuid.New(),
		})
		require.ErrorContains(t, err, "names no kind of entity")
	})
}

// fillJournal writes count entries naming actor, in one statement. Writing
// them through the store would be thousands of round trips for a case whose
// whole point is that the number is unreachable.
func fillJournal(ctx context.Context, t *testing.T, sqlDB *sql.DB, actor uuid.UUID, count int) {
	t.Helper()

	_, err := sqlDB.ExecContext(ctx, `
		INSERT INTO entity_journal (recorded_at, event, subject_type, subject, actor_type, actor)
		SELECT now(), 'created', 'ai_agent', gen_random_uuid(), 'workspace_agent', $1
		FROM generate_series(1, $2)
	`, actor, count)
	require.NoError(t, err)
}
