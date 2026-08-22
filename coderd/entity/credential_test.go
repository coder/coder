package entity_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/testutil"
)

// present builds a presentation of a declared credential. The verifier is
// coderd, which has no type of its own yet and so travels as a user.
func present(declared uuid.UUID, output string) entity.Presentation {
	return entity.Presentation{
		Declared:            declared,
		AuthenticatorOutput: output,
		Verifier:            entity.Ref{Type: entity.TypeUser, ID: uuid.New()},
	}
}

func TestCredentials(t *testing.T) {
	t.Parallel()

	t.Run("VerifiesTheIssuedCredential", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)
		require.NotEmpty(t, issued)

		ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
		require.NoError(t, err)
		require.True(t, ok, "the issued credential should verify")
	})

	t.Run("RefusesEverythingElse", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)

		for _, tc := range []struct {
			name      string
			declared  uuid.UUID
			presented string
		}{
			{"WrongAuthenticator", issued.ID, "not the issued authenticator"},
			{"EmptyAuthenticator", issued.ID, ""},
			{"UnknownCredential", uuid.New(), issued.Authenticator},
			{"UnknownCredentialAndOutput", uuid.New(), "anything"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Its own context. The parent's is canceled when the parent
				// function returns, which happens before parallel subtests run.
				ctx := testutil.Context(t, testutil.WaitShort)

				ok, err := entity.VerifyCredential(ctx, db, present(tc.declared, tc.presented))
				require.NoError(t, err)
				require.False(t, ok)
			})
		}
	})

	// An entity may hold several valid credentials at once, which is what the
	// table has no key in order to allow. Rotation without a moment of no valid
	// credential depends on it.
	t.Run("VerifiesAnyOfSeveralValidCredentials", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		first, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)
		second, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)
		require.NotEqual(t, first, second, "each issuance should mint its own")

		// Both remain valid, which is what lets a rotation overlap. Declaring
		// which is being presented is how the verifier knows which to check.
		for _, issued := range []entity.IssuedCredential{first, second} {
			ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
			require.NoError(t, err)
			require.True(t, ok)
		}
	})

	t.Run("RejectsAnUnknownHolderType", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		bad := entity.Ref{Type: "sandbox", ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		_, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: bad, Actor: actor})
		require.ErrorContains(t, err, "names no kind of entity")

		// Verification takes no holder. What it needs an identity for is the
		// verifier, which is the actor of the operation it records.
		_, err = entity.VerifyCredential(ctx, db, entity.Presentation{
			Declared:            uuid.New(),
			AuthenticatorOutput: "anything",
		})
		require.ErrorContains(t, err, "observed by a verifier")
	})

	t.Run("TheLedgerKeepsAHashAndNotTheAuthenticator", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialTypePassword, row.CredentialType)
		require.Equal(t, entity.CredentialStateValid, row.State)
		require.False(t, row.ExpiresAt.Valid, "nothing issues an expiry yet")

		// The digest is in the password type's own table, which the ledger row
		// names by carrying its type.
		password, err := db.GetCredentialPasswordByID(ctx, issued.ID)
		require.NoError(t, err, "a password credential should have a password row")
		require.NotEqual(t, issued.Authenticator, password.HashedAuthenticator,
			"the ledger must not hold what was handed out")
		require.Len(t, password.HashedAuthenticator, 64, "a SHA-256 digest in hex")
	})

	t.Run("ANullCredentialAlwaysVerifies", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{
			Holder: holder, Actor: actor, Type: entity.CredentialTypeNull,
		})
		require.NoError(t, err)
		require.Empty(t, issued.Authenticator, "a null credential hands out nothing")

		for _, presented := range []string{"", "anything at all"} {
			ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, presented))
			require.NoError(t, err)
			require.True(t, ok, "a null credential accepts whatever is presented")
		}
	})

	t.Run("RevocationInvalidatesAndJournals", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)

		ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
		require.NoError(t, err)
		require.True(t, ok, "it verifies before revocation")

		require.NoError(t, entity.RevokeCredential(ctx, db, issued.ID, actor))

		ok, err = entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
		require.NoError(t, err)
		require.False(t, ok, "it does not verify after revocation")

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialStateInvalid, row.State,
			"the row remains; a ledger keeps its retired rows")

		require.ErrorContains(t, entity.RevokeCredential(ctx, db, issued.ID, actor),
			"already invalid", "invalid is terminal")
	})
}

// The api_key type is the first whose issuance takes parameters, and so the
// first to write a line. These check that the line and the ledger row are
// written together and say the same thing, and that the type's parameters are
// required exactly for it.
func TestIssueAPIKeyCredential(t *testing.T) {
	t.Parallel()

	params := func(holder, actor entity.Ref) entity.IssueCredentialParams {
		return entity.IssueCredentialParams{
			Holder: holder,
			Actor:  actor,
			Type:   entity.CredentialTypeAPIKey,
			APIKey: &entity.APIKeyCredential{
				TokenName: "ai-ws-" + uuid.NewString(),
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{{Type: "workspace", ID: uuid.NewString()}},
			},
		}
	}

	t.Run("WritesALineAndALedgerRow", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		in := params(holder, actor)

		issued, err := entity.IssueCredential(ctx, db, in)
		require.NoError(t, err)
		require.NotEmpty(t, issued.Authenticator, "an api_key credential has a secret")

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.Equal(t, entity.CredentialTypeAPIKey, row.CredentialType)

		key, err := db.GetCredentialAPIKeyByID(ctx, issued.ID)
		require.NoError(t, err)
		require.Equal(t, in.APIKey.TokenName, key.TokenName)
		require.Equal(t, in.APIKey.Scopes, key.Scopes)
		require.Equal(t, in.APIKey.AllowList, key.AllowList)
		require.NotEqual(t, issued.Authenticator, key.HashedSecret,
			"the ledger must not hold what was handed out")

		// The line says what the issuance carried. It is a separate statement
		// from the ledger row, which says what the credential now is, and here
		// they agree because nothing has happened since.
		lines, err := db.GetCredentialLifecycleJournalAPIKeyLines(ctx, row.LifecyclePostingReference)
		require.NoError(t, err)
		require.Len(t, lines, 1, "one issuance carries one line")
		require.EqualValues(t, 0, lines[0].Line, "the only line of an entry is line zero")
		require.Equal(t, in.APIKey.TokenName, lines[0].TokenName)
		require.Equal(t, in.APIKey.Scopes, lines[0].Scopes)
		require.Equal(t, in.APIKey.AllowList, lines[0].AllowList)
	})

	t.Run("VerifiesItsOwnSecret", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		issued, err := entity.IssueCredential(ctx, db, params(holder, actor))
		require.NoError(t, err)

		ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
		require.NoError(t, err)
		require.True(t, ok, "the secret handed back should verify")

		ok, err = entity.VerifyCredential(ctx, db, present(issued.ID, "not-the-secret"))
		require.NoError(t, err)
		require.False(t, ok, "another secret should not")
	})

	t.Run("ParametersAreRequiredExactlyForTheType", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		bare := entity.IssueCredentialParams{Holder: holder, Actor: actor, Type: entity.CredentialTypeAPIKey}
		_, err := entity.IssueCredential(ctx, db, bare)
		require.ErrorContains(t, err, "given together or not at all",
			"an api_key credential without its parameters issues nothing")

		wrongType := params(holder, actor)
		wrongType.Type = entity.CredentialTypePassword
		_, err = entity.IssueCredential(ctx, db, wrongType)
		require.ErrorContains(t, err, "given together or not at all",
			"parameters for a type that does not take them are refused")

		empty := params(holder, actor)
		empty.APIKey.AllowList = database.AllowList{}
		_, err = entity.IssueCredential(ctx, db, empty)
		require.ErrorContains(t, err, "confers nothing")
	})
}

// The credential's second model. Its operations assign rather than transition,
// and both are observed, so these check what each assigns and what the entry
// records about who noticed.
func TestCredentialUse(t *testing.T) {
	t.Parallel()

	issue := func(t *testing.T, db database.Store, ctx context.Context) entity.IssuedCredential {
		t.Helper()
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{
			Holder: entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()},
			Actor:  entity.Ref{Type: entity.TypeUser, ID: uuid.New()},
		})
		require.NoError(t, err)
		return issued
	}

	t.Run("BothVariablesBeginUnassigned", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		issued := issue(t, db, ctx)

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.False(t, row.LastPresented.Valid, "never offered")
		require.False(t, row.LastUsed.Valid, "never accepted")
		require.False(t, row.UsePostingReference.Valid, "nothing has posted")
	})

	t.Run("AnAcceptedPresentationAssignsBoth", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		issued := issue(t, db, ctx)
		verifier := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		ok, err := entity.VerifyCredential(ctx, db, entity.Presentation{
			Declared:            issued.ID,
			AuthenticatorOutput: issued.Authenticator,
			Verifier:            verifier,
			AnnotationSource:    "unix socket",
		})
		require.NoError(t, err)
		require.True(t, ok)

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.True(t, row.LastPresented.Valid)
		require.True(t, row.LastUsed.Valid)
		require.Equal(t, row.LastPresented.Time, row.LastUsed.Time,
			"one presentation assigns one moment to both")

		entries, err := db.GetCredentialUseJournalEntriesBySubject(ctx, database.GetCredentialUseJournalEntriesBySubjectParams{
			Subject: issued.ID, Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, string(entity.EventPresentationAccepted), entries[0].Event)
		require.Equal(t, string(entity.TypeUser), entries[0].ActorType)
		require.Equal(t, verifier.ID, entries[0].Actor, "the actor is the verifier that noticed")
		require.Equal(t, "unix socket", entries[0].AnnotationSource.String)
		require.Equal(t, entries[0].EntryID, row.UsePostingReference.Int64,
			"the row should name the entry that posted to it")
	})

	t.Run("ARefusedPresentationAssignsOnlyPresented", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		issued := issue(t, db, ctx)

		ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, "not the authenticator"))
		require.NoError(t, err)
		require.False(t, ok)

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.True(t, row.LastPresented.Valid, "it was offered")
		require.False(t, row.LastUsed.Valid, "it was not accepted")

		entries, err := db.GetCredentialUseJournalEntriesBySubject(ctx, database.GetCredentialUseJournalEntriesBySubjectParams{
			Subject: issued.ID, Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, string(entity.EventPresentationRefused), entries[0].Event)
	})

	// The security value of the model: a credential offered after it stopped
	// being valid leaves a record naming that credential.
	t.Run("APresentationAfterRevocationIsRecorded", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		issued := issue(t, db, ctx)
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		require.NoError(t, entity.RevokeCredential(ctx, db, issued.ID, actor))

		ok, err := entity.VerifyCredential(ctx, db, present(issued.ID, issued.Authenticator))
		require.NoError(t, err)
		require.False(t, ok, "a revoked credential accepts nothing")

		entries, err := db.GetCredentialUseJournalEntriesBySubject(ctx, database.GetCredentialUseJournalEntriesBySubjectParams{
			Subject: issued.ID, Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, entries, 1, "the right authenticator offered too late is still recorded")
		require.Equal(t, string(entity.EventPresentationRefused), entries[0].Event)
	})

	// Posting is in journal order. An older entry arriving after a newer one
	// has posted affects no row, and that is correct rather than a failure.
	t.Run("AnOlderEntryDoesNotOverwriteANewerOne", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		issued := issue(t, db, ctx)

		newer, err := db.NextCredentialUseJournalEntryID(ctx)
		require.NoError(t, err)
		older, err := db.NextCredentialUseJournalEntryID(ctx)
		require.NoError(t, err)
		require.Greater(t, older, newer, "the sequence advances")
		// Post the higher identifier first, then try the lower.
		newer, older = older, newer

		late := dbtime.Now()
		_, err = db.PostCredentialPresentation(ctx, database.PostCredentialPresentationParams{
			ID: issued.ID, PresentedAt: sql.NullTime{Time: late, Valid: true},
			Accepted: true, EntryID: sql.NullInt64{Int64: newer, Valid: true},
		})
		require.NoError(t, err)

		_, err = db.PostCredentialPresentation(ctx, database.PostCredentialPresentationParams{
			ID: issued.ID, PresentedAt: sql.NullTime{Time: late.Add(-time.Hour), Valid: true},
			Accepted: false, EntryID: sql.NullInt64{Int64: older, Valid: true},
		})
		require.ErrorIs(t, err, sql.ErrNoRows, "an older entry posts to nothing")

		row, err := db.GetCredentialLedgerRowByID(ctx, issued.ID)
		require.NoError(t, err)
		require.Equal(t, newer, row.UsePostingReference.Int64, "the newer entry still holds the row")
	})
}
