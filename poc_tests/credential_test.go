package poctests_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
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

	t.Run("AnAIAgentWithNoUsersRowAuthenticates", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)
		systemCtx := dbauthz.AsSystemRestricted(ctx)

		// Created by this work, so no users row exists for it anywhere. That
		// is what the test is about: the server has to authenticate a party it
		// cannot look up in users.
		agent, err := entity.CreateAIAgent(systemCtx, db, entity.CreateAIAgentParams{
			Owner:        entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			CreationSite: entity.CreationSite{Type: entity.CreationSiteTypeWorkspace, ID: uuid.New()},
		})
		require.NoError(t, err)

		_, err = db.GetUserByID(systemCtx, agent.ID)
		require.ErrorIs(t, err, sql.ErrNoRows, "the agent must not be a user for this to prove anything")

		issued, err := entity.IssueCredential(systemCtx, db, entity.IssueCredentialParams{
			Holder: entity.Ref{Type: entity.TypeAIAgent, ID: agent.ID},
			Actor:  entity.Ref{Type: entity.TypeUser, ID: owner.UserID},
			Type:   entity.CredentialTypeAPIKey,
			APIKey: &entity.APIKeyCredential{
				TokenName: "wp5-acceptance",
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}},
			},
		})
		require.NoError(t, err)

		holder := codersdk.New(client.URL)
		holder.SetSessionToken(issued.Authenticator)

		// An endpoint authorized by the subject and asking for no user by name.
		// What is being proved is that the token was accepted, a subject was
		// built for a holder with no users row, and authorization ran against
		// it.
		_, err = holder.Workspaces(ctx, codersdk.WorkspaceFilter{})
		require.NoError(t, err, "the server must authenticate an AI agent that is not a user")

		// **Asserting a gap on purpose.** "me" is resolved by
		// coderd/httpmw/userparam.go through GetUserByID on the holder, and an
		// AI agent has no row there, so the request is refused after being
		// authenticated. Whether "me" should mean the owner, or mean nothing
		// for an agent, is a question this work has not answered. Recorded as
		// a failing expectation rather than fixed, so that the day it is
		// answered this test fails and somebody reads the answer.
		_, err = holder.User(ctx, codersdk.Me)
		require.Error(t, err, "resolving \"me\" for an agent has no answer yet")

		// **The worse half of the same gap, asserted deliberately.** Seven
		// places in coderd resolve "me" by taking the holder for a user id,
		// and most of them filter rather than fetch. So an agent asking for
		// its own workspaces is told it has none, with no error, which is
		// indistinguishable from having none. The CLI sends owner:me by
		// default, so nothing has to ask for this to happen.
		mine, err := holder.Workspaces(ctx, codersdk.WorkspaceFilter{Owner: codersdk.Me})
		require.NoError(t, err, "the silent case does not even fail")
		require.Empty(t, mine.Workspaces, "it answers none, which is not the same as cannot say")

		// Retirement lapses the credential, so the same token stops working.
		// This is the whole chain in one assertion: ledger state reaching an
		// HTTP response.
		require.NoError(t, entity.RetireAIAgent(systemCtx, db, agent.ID, entity.EventAIAgentKill,
			entity.Ref{Type: entity.TypeUser, ID: owner.UserID}, time.Time{}))

		_, err = holder.User(ctx, codersdk.Me)
		require.Error(t, err, "a retired agent's credential must stop authenticating")
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
