package poctests_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestCredentialFoundations is the acceptance test for work package WP4 in
// poc_audit/work_breakdown.md.
//
// The package's claim is that api key issuance can go through a journal. What
// makes that a claim about the running system rather than about a table is that
// the credential has to work: a token the journal minted is presented to the
// server over HTTP and the server has to accept it, using the authentication
// path it already had and that knows nothing of any ledger.
//
// The test therefore reaches past the entity package on purpose. Everything it
// asserts after issuance is the existing system's behavior, unchanged.
func TestCredentialFoundations(t *testing.T) {
	t.Parallel()

	t.Run("AnIssuedTokenAuthenticates", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Issuance writes the journal, the ledger, the type's own row, and the
		// api_keys row, in one transaction. Only the last of those is visible
		// to what follows.
		issued, err := entity.IssueCredential(dbauthz.AsSystemRestricted(ctx), db, entity.IssueCredentialParams{
			Holder: entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			Actor:  entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			Type:   entity.CredentialTypeAPIKey,
			APIKey: &entity.APIKeyCredential{
				TokenName: "wp4-acceptance",
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
			},
		})
		require.NoError(t, err)

		holder := codersdk.New(client.URL)
		holder.SetSessionToken(issued.Authenticator)

		me, err := holder.User(ctx, codersdk.Me)
		require.NoError(t, err, "the server must accept a token the journal minted")
		require.Equal(t, owner.UserID, me.ID, "and must resolve it to the holder")
	})

	t.Run("ARevokedCredentialStillAuthenticates", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		systemCtx := dbauthz.AsSystemRestricted(ctx)

		issued, err := entity.IssueCredential(systemCtx, db, entity.IssueCredentialParams{
			Holder: entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			Actor:  entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			Type:   entity.CredentialTypeAPIKey,
			APIKey: &entity.APIKeyCredential{
				TokenName: "wp4-divergence",
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
			},
		})
		require.NoError(t, err)
		require.NoError(t, entity.RevokeCredential(systemCtx, db, issued.ID,
			entity.Ref{Type: entity.TypeUser, ID: owner.UserID}))

		holder := codersdk.New(client.URL)
		holder.SetSessionToken(issued.Authenticator)

		// **This asserts a defect, deliberately.** The mirror is one way and
		// covers issuance alone, so the ledger says invalid and api_keys has
		// not been told. Recording it as a passing assertion means the day
		// revocation joins the mirror this test fails and has to be rewritten,
		// which is the notice we want. See "What this package does not do" in
		// poc_audit/work_breakdown.md.
		_, err = holder.User(ctx, codersdk.Me)
		require.NoError(t, err, "revocation does not reach api_keys yet")
	})
}
