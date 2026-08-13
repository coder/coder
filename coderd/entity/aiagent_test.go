package entity_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateAIAgent(t *testing.T) {
	t.Parallel()

	t.Run("WritesACreationEntry", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		actor := entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()}

		id, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{Actor: actor})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, id, "creation should mint an identity")

		entries := entriesFor(ctx, t, db, id)
		require.Len(t, entries, 1, "creation should write exactly one entry")

		got := entries[0]
		require.Equal(t, string(entity.EventCreated), got.Event)
		require.Equal(t, string(entity.TypeAIAgent), got.SubjectType)
		require.Equal(t, id, got.Subject, "the entry should name the agent that was created")
		require.Equal(t, string(entity.TypeWorkspaceAgent), got.ActorType)
		require.Equal(t, actor.ID, got.Actor, "the entry should name the actor that brought it about")
		require.NotZero(t, got.RecordedAt)
		require.NotZero(t, got.ID, "entries need distinct identifiers")
	})

	// The convention is that a lifecycle function joins the caller's
	// transaction when given one, so that creation can be made atomic with
	// work that is not creation. The observable consequence is that the entry
	// rolls back with the caller, which is what this checks.
	t.Run("JoinsTheCallersTransaction", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		errCallerFailed := xerrors.New("the caller's other work failed")

		var id uuid.UUID
		err := db.InTx(func(tx database.Store) error {
			var err error
			id, err = entity.CreateAIAgent(ctx, tx, entity.CreateAIAgentParams{
				Actor: entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()},
			})
			if err != nil {
				return err
			}
			return errCallerFailed
		}, nil)
		require.ErrorIs(t, err, errCallerFailed)

		require.Empty(t, entriesFor(ctx, t, db, id),
			"the entry should roll back with the transaction it joined")
	})

	t.Run("RejectsAnEntryWithNoActor", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{})
		require.ErrorContains(t, err, "actor")
	})
}

func entriesFor(ctx context.Context, t *testing.T, db database.Store, id uuid.UUID) []database.EntityJournal {
	t.Helper()

	entries, err := db.GetEntityJournalEntriesBySubject(ctx, database.GetEntityJournalEntriesBySubjectParams{
		SubjectType: string(entity.TypeAIAgent),
		Subject:     id,
	})
	require.NoError(t, err)
	return entries
}
