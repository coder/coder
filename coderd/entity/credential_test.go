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

		ok, err := entity.VerifyCredential(ctx, db, holder, issued.Authenticator)
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
			holder    entity.Ref
			presented string
		}{
			{"WrongAuthenticator", holder, "not the issued authenticator"},
			{"EmptyAuthenticator", holder, ""},
			{"CredentialOfAnother", entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}, issued.Authenticator},
			{"UnknownHolder", entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}, "anything"},
			{"WrongHolderType", entity.Ref{Type: entity.TypeUser, ID: holder.ID}, issued.Authenticator},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Its own context. The parent's is canceled when the parent
				// function returns, which happens before parallel subtests run.
				ctx := testutil.Context(t, testutil.WaitShort)

				ok, err := entity.VerifyCredential(ctx, db, tc.holder, tc.presented)
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

		for _, credential := range []string{first.Authenticator, second.Authenticator} {
			ok, err := entity.VerifyCredential(ctx, db, holder, credential)
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

		_, err = entity.VerifyCredential(ctx, db, bad, "anything")
		require.ErrorContains(t, err, "names no kind of entity")
	})

	t.Run("TheLedgerKeepsAHashAndNotTheAuthenticator", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		actor := entity.Ref{Type: entity.TypeUser, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, entity.IssueCredentialParams{Holder: holder, Actor: actor})
		require.NoError(t, err)

		row, err := db.GetCredentialLifecycleLedgerRowByID(ctx, issued.ID)
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
			ok, err := entity.VerifyCredential(ctx, db, holder, presented)
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

		ok, err := entity.VerifyCredential(ctx, db, holder, issued.Authenticator)
		require.NoError(t, err)
		require.True(t, ok, "it verifies before revocation")

		require.NoError(t, entity.RevokeCredential(ctx, db, issued.ID, actor))

		ok, err = entity.VerifyCredential(ctx, db, holder, issued.Authenticator)
		require.NoError(t, err)
		require.False(t, ok, "it does not verify after revocation")

		row, err := db.GetCredentialLifecycleLedgerRowByID(ctx, issued.ID)
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

		row, err := db.GetCredentialLifecycleLedgerRowByID(ctx, issued.ID)
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
		lines, err := db.GetCredentialLifecycleJournalAPIKeyLines(ctx, row.PostingReference)
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

		ok, err := entity.VerifyCredential(ctx, db, holder, issued.Authenticator)
		require.NoError(t, err)
		require.True(t, ok, "the secret handed back should verify")

		ok, err = entity.VerifyCredential(ctx, db, holder, "not-the-secret")
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
