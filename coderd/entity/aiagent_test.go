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
			Owner:  entity.Ref{Type: entity.TypeUser, ID: owner.ID},
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
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
		require.Equal(t, string(entity.EventAIAgentCreate), got.Event)
		require.Equal(t, id, got.Subject, "the entry should name the agent that was created")
		require.Equal(t, string(entity.TypeUser), got.ActorType,
			"creation is commanded by the owner, not by a relaying workspace_agent")
		require.Equal(t, owner.ID, got.Actor)
		require.False(t, got.RecordingDate.IsZero(), "an entry carries the recording date")
		require.False(t, got.EffectiveDate.IsZero(), "an entry carries the effective date")
		require.Equal(t, got.EntryID, row.PostingReference,
			"the ledger row should name the entry that posted to it")
	})

	t.Run("GrantsAuthorizationToTheOwner", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		owner := dbgen.User(t, db, database.User{})

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:  entity.Ref{Type: entity.TypeUser, ID: owner.ID},
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
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
	t.Run("RecordsTheOriginItWasCreatedIn", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		user := dbgen.User(t, db, database.User{})
		owner := entity.Ref{Type: entity.TypeUser, ID: user.ID}
		workspace := uuid.New()

		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:  owner,
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: workspace},
		})
		require.NoError(t, err)

		row, err := db.GetAIAgentLedgerRowByID(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, string(entity.OriginTypeWorkspace), row.OriginType)
		require.Equal(t, workspace, row.OriginID)

		// The line says what the creation carried. The ledger says what the
		// agent is. They agree here because nothing has happened since, and
		// they are separate statements even so.
		lines, err := db.GetAIAgentLifecycleJournalCreateLines(ctx, row.PostingReference)
		require.NoError(t, err)
		require.Len(t, lines, 1, "one creation carries one line")
		require.EqualValues(t, 0, lines[0].Line, "the only line of an entry is line zero")
		require.Equal(t, string(entity.OriginTypeWorkspace), lines[0].OriginType)
		require.Equal(t, workspace, lines[0].OriginID)
	})

	t.Run("RejectsCreationWithNoOrigin", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		user := dbgen.User(t, db, database.User{})
		owner := entity.Ref{Type: entity.TypeUser, ID: user.ID}

		_, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{Owner: owner})
		require.ErrorContains(t, err, "names no kind of thing")

		_, err = entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:  owner,
			Origin: entity.Origin{Type: entity.OriginTypeChat},
		})
		require.ErrorContains(t, err, "embodied in something")
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
				Owner:  entity.Ref{Type: entity.TypeUser, ID: owner.ID},
				Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
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
			Owner:  entity.Ref{Type: entity.TypeUser, ID: uuid.Nil},
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
		})
		require.ErrorContains(t, err, "belongs to a principal")
	})

	t.Run("RejectsAnOwnerOfAnUnknownKind", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:  entity.Ref{Type: "sandbox", ID: uuid.New()},
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
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

	newAgent := func(t *testing.T, ctx context.Context, db database.Store) (entity.NewAIAgent, entity.Ref) {
		t.Helper()
		user := dbgen.User(t, db, database.User{})
		owner := entity.Ref{Type: entity.TypeUser, ID: user.ID}
		created, err := entity.CreateAIAgent(ctx, db, entity.CreateAIAgentParams{
			Owner:  owner,
			Origin: entity.Origin{Type: entity.OriginTypeWorkspace, ID: uuid.New()},
		})
		require.NoError(t, err)
		return created, owner
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
			agent, owner := newAgent(t, ctx, db)
			id := agent.ID

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
			require.WithinDuration(t, happened, got.EffectiveDate, time.Second,
				"the effective date is when it happened, not when it was recorded")
			require.True(t, got.RecordingDate.After(got.EffectiveDate),
				"a late entry records an earlier event")
		})
	}

	// Retirement ends more than the agent. An authorization naming a party that
	// no longer exists cannot hold, and a credential authenticating one
	// authenticates nobody.
	t.Run("LapsesTheAuthorizationAndTheCredential", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)

		happened := dbtime.Now().Add(-time.Hour)
		require.NoError(t, entity.RetireAIAgent(ctx, db, agent.ID, entity.EventAIAgentKill, owner, happened))

		authorization, err := db.GetAuthorizationLedgerRowByID(ctx, agent.AuthorizationID)
		require.NoError(t, err)
		require.Equal(t, entity.StateTerminated, authorization.State)

		credential, err := db.GetCredentialLedgerRowByID(ctx, agent.CredentialID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialStateInvalid, credential.State)

		// Both entries are observed operations, so neither names the party who
		// commanded the retirement. Attributing them to the owner would say the
		// owner revoked what in fact lapsed.
		authEntries, err := db.GetAuthorizationLifecycleJournalEntriesBySubject(ctx,
			database.GetAuthorizationLifecycleJournalEntriesBySubjectParams{Subject: agent.AuthorizationID, Limit: 10})
		require.NoError(t, err)
		require.Len(t, authEntries, 2, "the grant and the lapse")
		lapse := authEntries[1]
		require.Equal(t, string(entity.EventAuthorizationLapse), lapse.Event)
		require.Equal(t, string(entity.SystemActor.Type), lapse.ActorType.String)
		require.Equal(t, entity.SystemActor.ID, lapse.Actor.UUID)
		require.NotEqual(t, owner.ID, lapse.Actor.UUID,
			"the party who ended the agent did not notice the consequence")
		require.WithinDuration(t, happened, lapse.EffectiveDate.Time, time.Second,
			"a lapse happens when the party ceased to exist, not when it was written")
		require.Equal(t, authorization.PostingReference, lapse.EntryID)

		credEntries, err := db.GetCredentialLifecycleJournalEntriesBySubject(ctx,
			database.GetCredentialLifecycleJournalEntriesBySubjectParams{Subject: agent.CredentialID, Limit: 10})
		require.NoError(t, err)
		require.Len(t, credEntries, 2, "the issuance and the lapse")
		credLapse := credEntries[1]
		require.Equal(t, string(entity.EventCredentialLapse), credLapse.Event)
		require.Equal(t, entity.SystemActor.ID, credLapse.Actor)
		require.WithinDuration(t, happened, credLapse.EffectiveDate, time.Second)
		require.Equal(t, credential.LifecyclePostingReference, credLapse.EntryID)
	})

	t.Run("LapsedCredentialsStopVerifying", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)

		verifier := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		present := entity.Presentation{
			Declared:            agent.CredentialID,
			AuthenticatorOutput: agent.Authenticator,
			Verifier:            verifier,
		}

		accepted, err := entity.VerifyCredential(ctx, db, present)
		require.NoError(t, err)
		require.True(t, accepted, "the credential works while the agent lives")

		require.NoError(t, entity.RetireAIAgent(ctx, db, agent.ID, entity.EventAIAgentFinish, owner, time.Time{}))

		// The point of the lapse, stated as behavior rather than as a state
		// column. A credential outliving its holder is a capability nobody
		// authorized.
		accepted, err = entity.VerifyCredential(ctx, db, present)
		require.NoError(t, err)
		require.False(t, accepted, "and stops the moment the agent is retired")
	})

	t.Run("LeavesAnotherAgentAlone", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		retired, owner := newAgent(t, ctx, db)
		survivor, _ := newAgent(t, ctx, db)

		require.NoError(t, entity.RetireAIAgent(ctx, db, retired.ID, entity.EventAIAgentKill, owner, time.Time{}))

		authorization, err := db.GetAuthorizationLedgerRowByID(ctx, survivor.AuthorizationID)
		require.NoError(t, err)
		require.Equal(t, entity.StateActive, authorization.State)

		credential, err := db.GetCredentialLedgerRowByID(ctx, survivor.CredentialID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialStateValid, credential.State)
	})

	t.Run("PassesOverWhatAlreadyEnded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)

		// A credential revoked before the agent ended is not a second thing to
		// end. One of them having gone early says nothing about the rest, so
		// retirement passes over it rather than failing.
		require.NoError(t, entity.RevokeCredential(ctx, db, agent.CredentialID, owner))
		require.NoError(t, entity.RetireAIAgent(ctx, db, agent.ID, entity.EventAIAgentKill, owner, time.Time{}))

		entries, err := db.GetCredentialLifecycleJournalEntriesBySubject(ctx,
			database.GetCredentialLifecycleJournalEntriesBySubjectParams{Subject: agent.CredentialID, Limit: 10})
		require.NoError(t, err)
		require.Len(t, entries, 2, "the issuance and the revocation, and no lapse on top")
		require.Equal(t, string(entity.EventCredentialRevoke), entries[1].Event,
			"the credential ended the way it actually ended")
	})

	t.Run("TheEndingsCommitTogether", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)
		errCallerFailed := xerrors.New("the caller's other work failed")

		err := db.InTx(func(tx database.Store) error {
			if err := entity.RetireAIAgent(ctx, tx, agent.ID, entity.EventAIAgentKill, owner, time.Time{}); err != nil {
				return err
			}
			return errCallerFailed
		}, nil)
		require.ErrorIs(t, err, errCallerFailed)

		// Three endings arising together, so none of them survives alone.
		row, err := db.GetAIAgentLedgerRowByID(ctx, agent.ID)
		require.NoError(t, err)
		require.Equal(t, entity.AIAgentStateActive, row.State)

		authorization, err := db.GetAuthorizationLedgerRowByID(ctx, agent.AuthorizationID)
		require.NoError(t, err)
		require.Equal(t, entity.StateActive, authorization.State)

		credential, err := db.GetCredentialLedgerRowByID(ctx, agent.CredentialID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialStateValid, credential.State)
	})

	t.Run("RetiredIsTerminal", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)
		id := agent.ID

		require.NoError(t, entity.RetireAIAgent(ctx, db, id, entity.EventAIAgentFinish, owner, time.Time{}))
		require.ErrorContains(t,
			entity.RetireAIAgent(ctx, db, id, entity.EventAIAgentKill, owner, time.Time{}),
			"already retired")
	})

	t.Run("RejectsAnEventThatDoesNotRetire", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		agent, owner := newAgent(t, ctx, db)
		id := agent.ID

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

// A name nothing stores. It has to say which kind of agent this is, that being
// the whole of what the stored name said beyond identifying it.
func TestDisplayName(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

	require.Equal(t, "ai-ws-"+id.String(), entity.DisplayName(entity.OriginTypeWorkspace, id))
	require.Equal(t, "ai-chat-"+id.String(), entity.DisplayName(entity.OriginTypeChat, id))

	require.NotEqual(t,
		entity.DisplayName(entity.OriginTypeWorkspace, id),
		entity.DisplayName(entity.OriginTypeChat, id),
		"a chat agent and a workspace agent must not read alike")

	// Same inputs, same name, every time. That is what lets nothing store it.
	require.Equal(t,
		entity.DisplayName(entity.OriginTypeWorkspace, id),
		entity.DisplayName(entity.OriginTypeWorkspace, id))
}
