package entity_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"strings"
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

	t.Run("VerifiesItsOwnToken", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		verifier := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		issued, err := entity.IssueCredential(ctx, db, params(holder, actor))
		require.NoError(t, err)

		// The round trip runs through the wire format rather than around it.
		// Presenting the token whole would present the wrong thing: the key id
		// is a declaration and only the secret half is an authenticator output.
		p, err := entity.APIKeyPresentation(ctx, db, issued.Authenticator, verifier, "test")
		require.NoError(t, err)
		require.Equal(t, issued.ID, p.Declared, "the key id names the credential")

		ok, err := entity.VerifyCredential(ctx, db, p)
		require.NoError(t, err)
		require.True(t, ok, "the token handed back should verify")

		keyID, _, found := strings.Cut(issued.Authenticator, "-")
		require.True(t, found)

		wrong, err := entity.APIKeyPresentation(ctx, db, keyID+"-"+strings.Repeat("x", 22), verifier, "test")
		require.NoError(t, err)
		ok, err = entity.VerifyCredential(ctx, db, wrong)
		require.NoError(t, err)
		require.False(t, ok, "another secret should not")

		unknown, err := entity.APIKeyPresentation(ctx, db, strings.Repeat("z", 10)+"-"+strings.Repeat("x", 22), verifier, "test")
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, unknown.Declared, "a key id naming nothing declares nothing")

		_, err = entity.APIKeyPresentation(ctx, db, "not-a-token", verifier, "test")
		require.Error(t, err, "a string that is not a token is no presentation")
	})

	t.Run("MirrorsIntoAPIKeys", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		in := params(holder, actor)

		issued, err := entity.IssueCredential(ctx, db, in)
		require.NoError(t, err)

		credential, err := db.GetCredentialAPIKeyByID(ctx, issued.ID)
		require.NoError(t, err)

		mirrored, err := db.GetAPIKeyByID(ctx, credential.KeyID)
		require.NoError(t, err)
		require.Equal(t, holder.ID, mirrored.HolderID.AsUserIDUnchecked(),
			"the two tables must agree on who holds the credential")
		require.Equal(t, database.HolderTypeAIAgent, mirrored.HolderType)
		require.Equal(t, in.APIKey.TokenName, mirrored.TokenName)
		require.Equal(t, in.APIKey.Scopes, mirrored.Scopes)
		require.Equal(t, in.APIKey.AllowList, mirrored.AllowList)
		require.Equal(t, database.LoginTypeToken, mirrored.LoginType)
		require.True(t, mirrored.ExpiresAt.After(time.Now()),
			"the mirror of a credential the ledger gives no expiry must not read as expired")

		// The same digest in two encodings. The ledger keeps hex and api_keys
		// keeps bytes, and a mirror that disagreed here would authenticate
		// nothing while looking correct.
		require.Equal(t, credential.HashedSecret, hex.EncodeToString(mirrored.HashedSecret))
	})

	t.Run("RefusesAHolderAPIKeysCannotHold", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		// The ledger records a credential for any kind of entity. api_keys
		// constrains its holder to two, so this one can be recorded and not
		// mirrored, and issuing it would leave the two disagreeing.
		holder := entity.Ref{Type: entity.TypeWorkspaceAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}

		_, err := entity.IssueCredential(ctx, db, params(holder, actor))
		require.ErrorContains(t, err, "api_keys holds no credential for a workspace_agent")
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

// TestDischargeSaysWhatEntailedIt asserts the shape of the entailing reference:
// exactly one form, never both and never neither. A discharge that cannot say
// what ended is refused rather than written, because an entailed operation whose
// cause is unrecorded is indistinguishable from one nobody bothered to explain.
func TestDischargeSaysWhatEntailedIt(t *testing.T) {
	t.Parallel()

	t.Run("NeitherFormIsRefused", func(t *testing.T) {
		t.Parallel()
		require.False(t, entity.EntailedBy{}.Valid())
	})

	t.Run("BothFormsAreRefused", func(t *testing.T) {
		t.Parallel()
		require.False(t, entity.EntailedBy{Entry: 1, Annotation: "a sandbox ended"}.Valid())
	})

	t.Run("EitherFormAlone", func(t *testing.T) {
		t.Parallel()
		require.True(t, entity.EntailedBy{Entry: 1}.Valid())
		require.True(t, entity.EntailedBy{Annotation: "a sandbox ended"}.Valid())
	})

	t.Run("DischargeRefusesAnUnexplainedEnding", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		db, _ := dbtestutil.NewDB(t)
		err := entity.DischargeCredential(ctx, db, uuid.New(), entity.EntailedBy{}, time.Time{})
		require.ErrorContains(t, err, "says what entailed it")
	})
}
