package coderd_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// createTestAIAgent creates an agent owned by the given user, which writes
// three entries in one transaction: the creation, the grant of authorization,
// and the issuance of a credential.
func createTestAIAgent(t *testing.T, db database.Store, ownerID uuid.UUID) entity.NewAIAgent {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	created, err := entity.CreateAIAgent(dbauthz.AsSystemRestricted(ctx), db, entity.CreateAIAgentParams{
		Owner:        entity.Ref{Type: entity.TypeUser, ID: ownerID},
		CreationSite: entity.CreationSite{Type: entity.CreationSiteTypeWorkspace, ID: uuid.New()},
	})
	require.NoError(t, err)
	return created
}

func TestAIAgentLedger(t *testing.T) {
	t.Parallel()

	t.Run("ListsTheOwnersAgents", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created := createTestAIAgent(t, db, owner.UserID)

		rows, err := client.AIAgentLedger(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, created.ID, rows[0].ID)
		require.Equal(t, codersdk.AIAgentStateActive, rows[0].State)
		require.Equal(t, owner.UserID, rows[0].OwnerID)
		require.Equal(t, codersdk.AIAgentCreationSiteTypeWorkspace, rows[0].CreationSiteType)
		require.NotEmpty(t, rows[0].DisplayName, "the name is computed, not stored")
	})

	t.Run("KeepsRetiredRows", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created := createTestAIAgent(t, db, owner.UserID)
		require.NoError(t, entity.RetireAIAgent(dbauthz.AsSystemRestricted(ctx), db, created.ID,
			entity.EventAIAgentKill, entity.Ref{Type: entity.TypeUser, ID: owner.UserID}, dbtime.Now()))

		// A ledger keeps its retired rows. A listing that dropped them would be
		// reporting the live population and calling it the ledger.
		rows, err := client.AIAgentLedger(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, codersdk.AIAgentStateRetired, rows[0].State)
	})
}

func TestAIAgentLifecycleJournals(t *testing.T) {
	t.Parallel()

	t.Run("CreationIsThreeEntriesInModelOrder", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created := createTestAIAgent(t, db, owner.UserID)

		response, err := client.AIAgentLifecycleJournals(ctx, created.ID)
		require.NoError(t, err)
		require.False(t, response.Truncated)
		require.Len(t, response.Entries, 3, "creation writes one entry to each of the three journals")

		// All three share a recording date, Postgres now() being transaction
		// start time, so nothing about the clock separates them. The order is
		// the model's: an entity that does not exist cannot be party to an
		// agency relation, and a credential is issued to a party that already
		// holds authority.
		require.Equal(t, codersdk.AIAgentJournalAIAgent, response.Entries[0].Journal)
		require.Equal(t, codersdk.AIAgentJournalAuthorization, response.Entries[1].Journal)
		require.Equal(t, codersdk.AIAgentJournalCredential, response.Entries[2].Journal)

		require.Equal(t, string(entity.EventAIAgentCreate), response.Entries[0].Event)
		require.Equal(t, string(entity.EventGrant), response.Entries[1].Event)
		require.Equal(t, string(entity.EventCredentialIssue), response.Entries[2].Event)

		require.Equal(t, created.ID, response.Entries[0].Subject)
		require.Equal(t, created.AuthorizationID, response.Entries[1].Subject)
		require.Equal(t, created.CredentialID, response.Entries[2].Subject)

		for _, entry := range response.Entries {
			require.NotNil(t, entry.ActorID, "creation is commanded, so every entry names who commanded it")
			require.Equal(t, owner.UserID, *entry.ActorID)
			require.False(t, entry.EffectiveDate.IsZero())
			require.False(t, entry.RecordingDate.IsZero())
		}
	})

	t.Run("EndedCredentialsAreStillReported", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created := createTestAIAgent(t, db, owner.UserID)
		require.NoError(t, entity.RetireAIAgent(dbauthz.AsSystemRestricted(ctx), db, created.ID,
			entity.EventAIAgentKill, entity.Ref{Type: entity.TypeUser, ID: owner.UserID}, dbtime.Now()))

		response, err := client.AIAgentLifecycleJournals(ctx, created.ID)
		require.NoError(t, err)

		// The credential is invalid after the retirement. Reading the holder's
		// credentials through the valid-only query would lose it, and its
		// ending with it, which is exactly what a record must not do.
		var credentialLapse, authorizationLapse *codersdk.AIAgentLifecycleJournalEntry
		for i, entry := range response.Entries {
			switch {
			case entry.Journal == codersdk.AIAgentJournalCredential && entry.Event == string(entity.EventCredentialLapse):
				credentialLapse = &response.Entries[i]
			case entry.Journal == codersdk.AIAgentJournalAuthorization && entry.Event == string(entity.EventAuthorizationLapse):
				authorizationLapse = &response.Entries[i]
			}
		}
		require.NotNil(t, credentialLapse, "the credential's ending should be reported")
		require.NotNil(t, authorizationLapse, "the authorization's ending should be reported")

		// A lapse is entailed, so the model says nobody performed it and no
		// actor is named. The code has not reached that position: both lapse
		// paths still write the superseded system actor, and correcting them
		// is deferred rather than forgotten.
		//
		// This asserts what the record currently says rather than what it
		// should say, so that the day the actor goes this fails and tells
		// whoever removed it that a reader of the record depended on it.
		require.NotNil(t, credentialLapse.ActorID,
			"lapse still names the superseded system actor")
		require.Equal(t, entity.SystemActor.ID, *credentialLapse.ActorID)

		// Entry level values are written once on line zero of a denormalized
		// journal, so a reader has to carry them forward. A zero date here
		// would mean that was not done.
		require.False(t, authorizationLapse.EffectiveDate.IsZero(),
			"line zero's effective date should be carried onto later lines")
	})

	t.Run("EveryLineOfAMultilineEntryCarriesItsEntryValues", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		sysCtx := dbauthz.AsSystemRestricted(ctx)
		ownerRef := entity.Ref{Type: entity.TypeUser, ID: owner.UserID}

		created := createTestAIAgent(t, db, owner.UserID)
		_, err := entity.GrantUniversalAuthorization(sysCtx, db, entity.GrantParams{
			Principal: ownerRef,
			Agent:     entity.Ref{Type: entity.TypeAIAgent, ID: created.ID},
		})
		require.NoError(t, err)

		// Retiring ends both authorizations as one event, so the entry has a
		// line per authorization and each line has a different subject.
		require.NoError(t, entity.RetireAIAgent(sysCtx, db, created.ID,
			entity.EventAIAgentKill, ownerRef, dbtime.Now()))

		response, err := client.AIAgentLifecycleJournals(ctx, created.ID)
		require.NoError(t, err)

		var lapses []codersdk.AIAgentLifecycleJournalEntry
		for _, entry := range response.Entries {
			if entry.Journal == codersdk.AIAgentJournalAuthorization &&
				entry.Event == string(entity.EventAuthorizationLapse) {
				lapses = append(lapses, entry)
			}
		}
		require.Len(t, lapses, 2, "both authorizations should have ended")

		// The journal is denormalized, so entry level values live on line zero,
		// and the lines of one entry can have different subjects. Reading by
		// subject can therefore return a line above zero with no line zero
		// beside it to take those values from. Every line must still carry
		// them, or an entry sorts to the beginning of time and reports no
		// actor where its sibling reports one.
		for _, lapse := range lapses {
			require.False(t, lapse.EffectiveDate.IsZero(),
				"line %d lost its entry's effective date", lapse.Line)
			require.False(t, lapse.RecordingDate.IsZero(),
				"line %d lost its entry's recording date", lapse.Line)
			require.NotNil(t, lapse.ActorID,
				"line %d lost its entry's actor", lapse.Line)
		}
		require.Equal(t, lapses[0].EntryID, lapses[1].EntryID,
			"one ending is one event, so both lines share an entry")
		require.Equal(t, *lapses[0].ActorID, *lapses[1].ActorID)
	})
}

func TestAIAgentRecordAuthorization(t *testing.T) {
	t.Parallel()

	t.Run("AnotherUserIsRefused", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		other, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		created := createTestAIAgent(t, db, owner.UserID)

		_, err := other.AIAgentLifecycleJournals(ctx, created.ID)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 404, sdkErr.StatusCode(), "an agent someone else owns is not disclosed")

		// The owner reads it without trouble, so the refusal is about the
		// requester and not about the route.
		response, err := client.AIAgentLifecycleJournals(ctx, created.ID)
		require.NoError(t, err)
		require.NotEmpty(t, response.Entries)
	})

	t.Run("AnUnknownAgentIsNotFound", func(t *testing.T) {
		t.Parallel()

		client, _ := coderdtest.NewWithDatabase(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.AIAgentLifecycleJournals(ctx, uuid.New())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, 404, sdkErr.StatusCode())
	})
}

func TestAIAgentSandboxLogs(t *testing.T) {
	t.Parallel()

	client, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	ctx := testutil.Context(t, testutil.WaitLong)
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	mine := createTestAIAgent(t, db, owner.UserID)
	theirs := createTestAIAgent(t, db, owner.UserID)

	started := dbtime.Now().Add(-time.Hour)
	for _, agentID := range []uuid.UUID{mine.ID, theirs.ID} {
		sessionID := uuid.New()
		_, err := db.UpsertAISandboxSession(sysCtx, database.UpsertAISandboxSessionParams{
			ID:                sessionID,
			WorkspaceID:       uuid.New(),
			ReporterAgentID:   uuid.New(),
			ConfinedAgentID:   uuid.New(),
			AIAgentID:         agentID,
			SponsorUserID:     owner.UserID,
			EgressEnforcement: "forced",
			StartedAt:         started,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		_, err = db.InsertAISandboxNetworkEvents(sysCtx, database.InsertAISandboxNetworkEventsParams{
			SessionID:      []uuid.UUID{sessionID},
			OccurredAt:     []time.Time{started},
			Protocol:       []string{"tcp"},
			Host:           []string{"example.invalid"},
			Port:           []int32{443},
			Action:         []string{"denied"},
			PolicyRevision: []int64{0},
			AIAgentID:      []uuid.UUID{agentID},
			SponsorUserID:  []uuid.UUID{owner.UserID},
			CreatedAt:      []time.Time{dbtime.Now()},
		})
		require.NoError(t, err)
	}

	// Both logs are read by the agent's own attribution snapshot, so one
	// agent's record never shows another's.
	sessions, err := client.AIAgentSandboxSessionsLog(ctx, mine.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, mine.ID, sessions[0].AIAgentID)

	events, err := client.AIAgentNetworkEventsLog(ctx, mine.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, codersdk.AISandboxNetworkEventActionDenied, events[0].Action)
	require.Equal(t, "example.invalid", events[0].Host)
}
