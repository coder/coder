package entity_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateAIAgent(t *testing.T) {
	t.Parallel()

	t.Run("WritesACreationEntry", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		owner := dbgen.User(t, db, database.User{})

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner: entity.Ref{Type: entity.TypeUser, ID: owner.ID},
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.ID, "creation should mint an identity")
		require.NotEmpty(t, created.Authenticator, "creation should issue a credential")
		require.NotEqual(t, uuid.Nil, created.CredentialID, "the credential should be identified")

		id := created.ID
		row, err := db.GetAIAgentLedgerRowByID(ctx, id)
		require.NoError(t, err, "the minted identity should name a ledger row")
		require.Equal(t, string(entity.TypeUser), row.OwnerType)
		require.Equal(t, owner.ID, row.OwnerID, "the AI agent should belong to its principal")
		require.Equal(t, entity.AIAgentStateActive, row.State)

		verified, err := entity.VerifyCredential(ctx, db, entity.Presentation{
			Declared:            created.CredentialID,
			AuthenticatorOutput: created.Authenticator,
			Verifier:            entity.Ref{Type: entity.TypeUser, ID: owner.ID},
		})
		require.NoError(t, err)
		require.True(t, verified, "the credential handed back should verify")

		entries := entriesFor(ctx, t, db, id)
		require.Len(t, entries, 1, "creation should write exactly one entry")

		got := entries[0]
		require.EqualValues(t, 0, got.Line, "the only line of an entry is line zero")
		require.Equal(t, string(entity.EventAIAgentCreate), got.Event)
		require.Equal(t, id, got.Subject, "the entry should name the agent that was created")
		require.Equal(t, string(entity.TypeUser), got.ActorType.String,
			"creation is commanded by the owner, not by a relaying workspace_agent")
		require.Equal(t, owner.ID, got.Actor.UUID)
		require.True(t, got.RecordingDate.Valid, "line zero carries the recording date")
		require.True(t, got.EffectiveDate.Valid, "line zero carries the effective date")
		require.Equal(t, got.EntryID, row.PostingReference,
			"the ledger row should name the entry that posted to it")
	})

	t.Run("GrantsAuthorizationToTheOwner", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		owner := dbgen.User(t, db, database.User{})

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner: entity.Ref{Type: entity.TypeUser, ID: owner.ID},
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.AuthorizationID, "creation should grant authorization")

		row, err := db.GetAuthorizationLedgerRowByID(ctx, created.AuthorizationID)
		require.NoError(t, err, "the grant should have posted to the ledger")
		require.Equal(t, string(entity.TypeUser), row.PrincipalType)
		require.Equal(t, owner.ID, row.PrincipalID, "the principal is the owner, not the relaying agent")
		require.Equal(t, string(entity.TypeAIAgent), row.AgentType)
		require.Equal(t, created.ID, row.AgentID, "the agent is the AI agent just created")
		require.Equal(t, entity.UniversalScope, row.Scope, "the proof of concept grants universally")
		require.Equal(t, entity.StateActive, row.State)

		entries, err := db.GetAuthorizationLifecycleJournalEntriesBySubject(ctx, database.GetAuthorizationLifecycleJournalEntriesBySubjectParams{
			Subject: created.AuthorizationID,
			Limit:   10,
		})
		require.NoError(t, err)
		require.Len(t, entries, 1, "a grant is one entry of one line")

		got := entries[0]
		require.EqualValues(t, 0, got.Line, "the only line of an entry is line zero")
		require.Equal(t, string(entity.EventGrant), got.Event)
		require.Equal(t, created.AuthorizationID, got.Subject, "the entry names the authorization")
		require.Equal(t, string(entity.TypeUser), got.ActorType.String,
			"a grant is an act of the principal, so the actor is the owner")
		require.Equal(t, owner.ID, got.Actor.UUID)
		require.True(t, got.RecordingDate.Valid, "line zero carries the recording date")
		require.True(t, got.EffectiveDate.Valid, "line zero carries the effective date")
		require.Equal(t, got.EntryID, row.PostingReference,
			"the ledger row should name the entry that posted to it")
	})

	// The convention is that a lifecycle function joins the caller's
	// transaction when given one, so that creation can be made atomic with
	// work that is not creation. The observable consequence is that the entry
	// rolls back with the caller, which is what this checks.
	t.Run("JoinsTheCallersTransaction", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		owner := dbgen.User(t, db, database.User{})
		errCallerFailed := xerrors.New("the caller's other work failed")

		var id uuid.UUID
		err := db.InTx(func(tx database.Store) error {
			var err error
			created, err := entity.CreateAIAgent(ctx, tx, entity.CreateAIAgentParams{
				Owner: entity.Ref{Type: entity.TypeUser, ID: owner.ID},
			})
			if err != nil {
				return err
			}
			id = created.ID
			return errCallerFailed
		}, nil)
		require.ErrorIs(t, err, errCallerFailed)

		require.Empty(t, entriesFor(ctx, t, db, id),
			"the entry should roll back with the transaction it joined")

		// The row and its entry commit together or not at all, so neither
		// should have survived.
		_, err = db.GetAIAgentLedgerRowByID(ctx, id)
		require.ErrorIs(t, err, sql.ErrNoRows,
			"the AI agent should roll back with the entry accounting for it")

		credentials, err := db.GetValidCredentialsByHolder(ctx, database.GetValidCredentialsByHolderParams{
			HolderType: string(entity.TypeAIAgent),
			HolderID:   id,
		})
		require.NoError(t, err)
		require.Empty(t, credentials, "the credential should roll back with the rest")
	})

	t.Run("RejectsCreationWithNoOwner", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner: entity.Ref{Type: entity.TypeUser, ID: uuid.Nil},
		})
		require.ErrorContains(t, err, "belongs to a principal")
	})

	t.Run("RejectsAnOwnerOfAnUnknownKind", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner: entity.Ref{Type: "sandbox", ID: uuid.New()},
		})
		require.ErrorContains(t, err, "names no kind of entity")
	})
}

// Nothing in the running system retires an AI agent yet, and whether anything
// will during the proof of concept is undecided. This exists so that whoever
// wires it in finds it works the first time, and because it is the only thing
// that makes the lapse transitions of the other two machines reachable at all.
func TestRetireAIAgent(t *testing.T) {
	t.Parallel()

	newAgent := func(t *testing.T, ctx context.Context, db database.Store) (uuid.UUID, entity.Ref) {
		t.Helper()
		user := dbgen.User(t, db, database.User{})
		owner := entity.Ref{Type: entity.TypeUser, ID: user.ID}
		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{Owner: owner})
		require.NoError(t, err)
		return created.ID, owner
	}

	// finish and kill reach the same state, so the entry is the only thing
	// that says which happened. That is the whole reason they are two
	// transitions rather than one.
	for _, tc := range []struct {
		name  string
		event entity.Event
	}{
		{"Finish", entity.EventAIAgentFinish},
		{"Kill", entity.EventAIAgentKill},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			id, owner := newAgent(t, ctx, db)

			before, err := db.GetAIAgentLedgerRowByID(ctx, id)
			require.NoError(t, err)

			// An observed transition may be recorded long after it happened,
			// so the effective date is given and is not the recording date.
			happened := dbtime.Now().Add(-time.Hour)
			require.NoError(t, entity.RetireAIAgent(ctx, db, id, tc.event, owner, happened))

			after, err := db.GetAIAgentLedgerRowByID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, entity.AIAgentStateRetired, after.State,
				"the row remains; a ledger keeps its retired rows")
			require.NotEqual(t, before.PostingReference, after.PostingReference,
				"the row should name the entry that retired it")

			entries := entriesFor(ctx, t, db, id)
			require.Len(t, entries, 2, "creation and retirement")
			got := entries[1]
			require.Equal(t, string(tc.event), got.Event,
				"which way it ended is carried by the transition, the state being the same either way")
			require.Equal(t, after.PostingReference, got.EntryID)
			require.WithinDuration(t, happened, got.EffectiveDate.Time, time.Second,
				"the effective date is when it happened, not when it was recorded")
			require.True(t, got.RecordingDate.Time.After(got.EffectiveDate.Time),
				"a late entry records an earlier event")
		})
	}

	t.Run("RetiredIsTerminal", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		id, owner := newAgent(t, ctx, db)

		require.NoError(t, entity.RetireAIAgent(ctx, db, id, entity.EventAIAgentFinish, owner, time.Time{}))
		require.ErrorContains(t,
			entity.RetireAIAgent(ctx, db, id, entity.EventAIAgentKill, owner, time.Time{}),
			"already retired")
	})

	t.Run("RejectsAnEventThatDoesNotRetire", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		id, owner := newAgent(t, ctx, db)

		require.ErrorContains(t,
			entity.RetireAIAgent(ctx, db, id, entity.EventAIAgentCreate, owner, time.Time{}),
			"does not retire")
	})
}

func entriesFor(ctx context.Context, t *testing.T, db database.Store, id uuid.UUID) []database.AIAgentLifecycleJournal {
	t.Helper()

	entries, err := db.GetAIAgentLifecycleEntriesBySubject(ctx, database.GetAIAgentLifecycleEntriesBySubjectParams{
		Subject: id,
		Limit:   10,
	})
	require.NoError(t, err)
	return entries
}
