package entity_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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

		owner := dbgen.User(t, db, database.User{})
		actor := entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()}

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			OwnerID: owner.ID,
			Actor:   actor,
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.ID, "creation should mint an identity")
		require.NotEmpty(t, created.Authenticator, "creation should issue a credential")
		require.NotEqual(t, uuid.Nil, created.CredentialID, "the credential should be identified")

		id := created.ID
		agent, err := db.GetEntityAIAgentByID(ctx, id)
		require.NoError(t, err, "the minted identity should name a row")
		require.Equal(t, owner.ID, agent.OwnerID, "the AI agent should belong to its principal")

		verified, err := entity.VerifyCredential(ctx, db, entity.Ref{Type: entity.TypeAIAgent, ID: id}, created.Authenticator)
		require.NoError(t, err)
		require.True(t, verified, "the credential handed back should verify")

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
	t.Run("GrantsAuthorizationToTheOwner", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		owner := dbgen.User(t, db, database.User{})
		relay := entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()}

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			OwnerID: owner.ID,
			Actor:   relay,
		})
		require.NoError(t, err)
		require.NotEqual(t, uuid.Nil, created.AuthorizationID, "creation should grant authorization")

		row, err := db.GetAuthorizationLifecycleLedgerRowByID(ctx, created.AuthorizationID)
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
				OwnerID: owner.ID,
				Actor:   entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()},
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
		_, err = db.GetEntityAIAgentByID(ctx, id)
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
			Actor: entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()},
		})
		require.ErrorContains(t, err, "owner")
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

	entries, err := entity.LifecycleEntriesBySubject(ctx, testutil.Logger(t), db, entity.Ref{
		Type: entity.TypeAIAgent,
		ID:   id,
	})
	require.NoError(t, err)
	return entries
}
