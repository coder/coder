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

		issued, err := entity.IssueCredential(ctx, db, holder)
		require.NoError(t, err)
		require.NotEmpty(t, issued)

		ok, err := entity.VerifyCredential(ctx, db, holder, issued)
		require.NoError(t, err)
		require.True(t, ok, "the issued credential should verify")
	})

	t.Run("RefusesEverythingElse", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, holder)
		require.NoError(t, err)

		for _, tc := range []struct {
			name      string
			holder    entity.Ref
			presented string
		}{
			{"WrongCredential", holder, "not the issued credential"},
			{"EmptyCredential", holder, ""},
			{"CredentialOfAnother", entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}, issued},
			{"UnknownHolder", entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}, "anything"},
			{"WrongHolderType", entity.Ref{Type: entity.TypeUser, ID: holder.ID}, issued},
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

		first, err := entity.IssueCredential(ctx, db, holder)
		require.NoError(t, err)
		second, err := entity.IssueCredential(ctx, db, holder)
		require.NoError(t, err)
		require.NotEqual(t, first, second, "each issuance should mint its own")

		for _, credential := range []string{first, second} {
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

		_, err := entity.IssueCredential(ctx, db, bad)
		require.ErrorContains(t, err, "names no kind of entity")

		_, err = entity.VerifyCredential(ctx, db, bad, "anything")
		require.ErrorContains(t, err, "names no kind of entity")
	})

	// Revocation deletes the row. There is no revoke function yet, so this
	// exercises the property the table's shape gives: a credential that is not
	// present does not verify.
	t.Run("ACredentialNoLongerPresentDoesNotVerify", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		holder := entity.Ref{Type: entity.TypeAIAgent, ID: uuid.New()}
		issued, err := entity.IssueCredential(ctx, db, holder)
		require.NoError(t, err)

		stored, err := db.GetValidCredentialsByActor(ctx, database.GetValidCredentialsByActorParams{
			ActorType: string(holder.Type),
			Actor:     holder.ID,
		})
		require.NoError(t, err)
		require.Len(t, stored, 1)
		require.Equal(t, issued, stored[0].Password,
			"the stored credential is the one that was issued, in plaintext, which is a PoC cheat")
	})
}
