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
)

// TestMCPServerUserTokensAuth exercises the owner-personal gating of
// mcp_server_user_tokens against the real RBAC authorizer, which the
// FakeAuthorizer-based method suite cannot do.
func TestMCPServerUserTokensAuth(t *testing.T) {
	t.Parallel()

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
				context.Background(),
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

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			// A per-case config keeps parallel subtests from sharing
			// token rows; the raw store seeds bypassing authz.
			cfg := dbgen.MCPServerConfig(t, store, database.MCPServerConfig{
				OrganizationID: org.ID,
			})
			_, err := store.UpsertMCPServerUserToken(context.Background(), database.UpsertMCPServerUserTokenParams{
				MCPServerConfigID: cfg.ID,
				UserID:            owner.ID,
				AccessToken:       "seed-access-token",
				TokenType:         "bearer",
			})
			require.NoError(t, err)

			ctx := dbauthz.As(context.Background(), tc.Subject)

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
