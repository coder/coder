package entity_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

func TestType(t *testing.T) {
	t.Parallel()

	// The set is closed, so this doubles as the list of what is in it.
	for _, valid := range []entity.Type{
		entity.TypeAIAgent,
		entity.TypeWorkspaceAgent,
		entity.TypeUser,
	} {
		require.True(t, valid.Valid(), "%q should be a known type", valid)
	}

	for _, invalid := range []entity.Type{
		"",
		"sandbox",
		"agent",
		"AI_AGENT",
		"ai_agents",
	} {
		require.False(t, invalid.Valid(), "%q should not be a known type", invalid)
	}
}

func TestAppendEntry(t *testing.T) {
	t.Parallel()

	// A type naming no kind of entity severs an entry from the thing it is
	// about, so it is refused rather than stored and puzzled over later.
	for _, tc := range []struct {
		name  string
		entry entity.Entry
	}{
		{
			name: "UnknownSubjectType",
			entry: entity.Entry{
				Event:   entity.EventCreated,
				Subject: entity.Ref{Type: "sandbox", ID: uuid.New()},
				Actor:   entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()},
			},
		},
		{
			name: "UnknownActorType",
			entry: entity.Entry{
				Event:   entity.EventCreated,
				Subject: entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()},
				Actor:   entity.Ref{Type: "nobody", ID: uuid.New()},
			},
		},
		{
			name: "AbsentActorType",
			entry: entity.Entry{
				Event:   entity.EventCreated,
				Subject: entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()},
				Actor:   entity.Ref{ID: uuid.New()},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitShort)

			_, err := entity.AppendEntry(ctx, db, tc.entry)
			require.ErrorContains(t, err, "names no kind of entity")

			// A refusal must leave nothing behind, or the journal ends up
			// holding the entries it just rejected.
			entries, err := db.GetEntityJournalEntriesBySubject(ctx, database.GetEntityJournalEntriesBySubjectParams{
				SubjectType: string(tc.entry.Subject.Type),
				Subject:     tc.entry.Subject.ID,
			})
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}
