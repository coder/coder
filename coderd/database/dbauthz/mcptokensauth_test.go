package dbauthz_test

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/testutil"
)

// TestMCPServerUserTokensAuth exercises the owner-personal gating of
// mcp_server_user_tokens against the real RBAC authorizer, which the
// FakeAuthorizer-based method suite cannot do.
func TestMCPServerUserTokensAuth(t *testing.T) {
	t.Parallel()

	setupCtx := testutil.Context(t, testutil.WaitLong)
	authz := rbac.NewAuthorizer(prometheus.NewRegistry())
	store, _ := dbtestutil.NewDB(t)
	db := dbauthz.New(store, authz, slogtest.Make(t, &slogtest.Options{
		IgnoreErrors: true,
	}), coderdtest.AccessControlStorePointer())

	org := dbgen.Organization(t, store, database.Organization{})
	otherOrg := dbgen.Organization(t, store, database.Organization{})
	owner := dbgen.User(t, store, database.User{})
	stranger := dbgen.User(t, store, database.User{})
	otherOrgAdmin := dbgen.User(t, store, database.User{})

	memberSubject := func(userID string) rbac.Subject {
		return rbac.Subject{
			ID: userID,
			Roles: must(rolestore.Expand(
				setupCtx,
				store,
				[]rbac.RoleIdentifier{rbac.RoleMember(), rbac.ScopedRoleOrgMember(org.ID)},
			)),
			Groups: []string{},
			Scope:  rbac.ExpandableScope(rbac.ScopeAll),
		}
	}

	requireAuthorized := func(t *testing.T, err error) {
		t.Helper()
		require.NoError(t, err)
	}
	requireNotAuthorized := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		require.True(t, dbauthz.IsNotAuthorizedError(err), "expected authorization failure, got: %v", err)
	}

	testCases := []struct {
		Name    string
		Subject rbac.Subject
		Check   func(t *testing.T, err error)
	}{
		{
			Name:    "Owner",
			Subject: memberSubject(owner.ID.String()),
			Check:   requireAuthorized,
		},
		{
			Name:    "Stranger",
			Subject: memberSubject(stranger.ID.String()),
			Check:   requireNotAuthorized,
		},
		{
			Name: "OtherOrgAdmin",
			Subject: rbac.Subject{
				ID:     otherOrgAdmin.ID.String(),
				Roles:  rbac.Roles(must(rbac.RoleIdentifiers{rbac.RoleMember(), rbac.ScopedRoleOrgAdmin(otherOrg.ID)}.Expand())),
				Groups: []string{},
				Scope:  rbac.ExpandableScope(rbac.ScopeAll),
			},
			Check: requireNotAuthorized,
		},
	}

	t.Run("ChatdSubjects", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		cfg := dbgen.MCPServerConfig(t, store, database.MCPServerConfig{
			OrganizationID: org.ID,
		})
		token, err := store.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            owner.ID,
			AccessToken:       "chatd-access-token",
			TokenType:         "bearer",
		})
		require.NoError(t, err)

		markFailure := func(ctx context.Context) error {
			_, err := db.MarkMCPServerUserTokenRefreshFailure(ctx, database.MarkMCPServerUserTokenRefreshFailureParams{
				ID:                        token.ID,
				UpdatedAt:                 token.UpdatedAt,
				OauthRefreshFailureReason: "test",
			})
			return err
		}

		// The daemon-wide chatd subject may read tokens but must not
		// hold personal-write access anywhere.
		chatdCtx := dbauthz.AsChatd(ctx)
		_, err = db.GetMCPServerUserToken(chatdCtx, database.GetMCPServerUserTokenParams{
			MCPServerConfigID: cfg.ID,
			UserID:            owner.ID,
		})
		requireAuthorized(t, err)
		requireNotAuthorized(t, markFailure(chatdCtx))

		// The token-owner subject is write-only: reads belong to the
		// daemon-wide chatd subject.
		_, err = db.GetMCPServerUserToken(
			dbauthz.AsChatdTokenOwner(ctx, owner.ID),
			database.GetMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
			},
		)
		requireNotAuthorized(t, err)

		// The per-user token-owner subject writes only its own
		// owner's rows.
		requireNotAuthorized(t, markFailure(dbauthz.AsChatdTokenOwner(ctx, stranger.ID)))
		require.NoError(t, markFailure(dbauthz.AsChatdTokenOwner(ctx, owner.ID)))
	})

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			// A per-case config keeps parallel subtests from sharing
			// token rows; the raw store seeds bypassing authz.
			testCtx := testutil.Context(t, testutil.WaitLong)
			cfg := dbgen.MCPServerConfig(t, store, database.MCPServerConfig{
				OrganizationID: org.ID,
			})
			_, err := store.UpsertMCPServerUserToken(testCtx, database.UpsertMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
				AccessToken:       "seed-access-token",
				TokenType:         "bearer",
			})
			require.NoError(t, err)

			ctx := dbauthz.As(testCtx, tc.Subject)

			_, err = db.GetMCPServerUserToken(ctx, database.GetMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
			})
			tc.Check(t, err)

			_, err = db.UpsertMCPServerUserToken(ctx, database.UpsertMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
				AccessToken:       "updated-access-token",
				TokenType:         "bearer",
			})
			tc.Check(t, err)

			err = db.DeleteMCPServerUserToken(ctx, database.DeleteMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
			})
			tc.Check(t, err)
		})
	}
}
