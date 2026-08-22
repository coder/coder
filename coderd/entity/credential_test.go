package entity_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
