package coderd_test

import (
	"context"
	"database/sql"
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
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestMCPServerConfigListReadContracts pins the read contracts of the
// authorized MCP server config list while the B1 fallback window is open.
// Exercised through the dbauthz boundary: the org-scoped read grants are
// HTTP-ineffective until the B3 org-scoped routes exist, so the contracts
// live at the database authorization layer.
//
// Org admin and org auditor read only their own org's configs (including
// disabled ones) through the authorized list. A custom site role holding
// only mcp_server_config:read reads across orgs (site scope), and a plain
// org member reads only their own org via the temporary member grant.
//
//nolint:tparallel,paralleltest // Subtests share one seeded database.
func TestMCPServerConfigListReadContracts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawStore, _, rawSQLDB := dbtestutil.NewDBWithSQLDB(t)
	adminClient := coderdtest.New(t, &coderdtest.Options{Database: rawStore})
	db := rawStore
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	defaultOrgID := firstUser.OrganizationID

	// A second organization with a config that org-scoped readers of the
	// default org must not see.
	secondOrg := dbgen.Organization(t, db, database.Organization{})

	// Seed one enabled and one disabled config in the default org (the
	// create handler seeds the default org during the window), and one
	// enabled config in the second org directly in the DB.
	enabledDefault := createMCPServerConfig(t, adminClient, "enabled-default", true)
	disabledDefault := createMCPServerConfig(t, adminClient, "disabled-default", false)
	otherOrgConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: secondOrg.ID,
		Enabled:        true,
	})

	authzDB := func() database.Store {
		return dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), coderdtest.AccessControlStorePointer())
	}

	// authorizedSlugs returns the slugs the given user sees through the
	// authorized list under their full (db-backed) subject.
	authorizedSlugs := func(user codersdk.User) []string {
		subject := coderdtest.AuthzUserSubjectWithDB(ctx, t, db, user)
		cfgs, err := authzDB().GetMCPServerConfigs(dbauthz.As(ctx, subject))
		require.NoError(t, err)
		out := make([]string, 0, len(cfgs))
		for _, c := range cfgs {
			out = append(out, c.Slug)
		}
		return out
	}

	t.Run("OrgAdminSeesOwnOrgOnly", func(t *testing.T) {
		_, orgAdmin := coderdtest.CreateAnotherUser(t, adminClient, defaultOrgID, rbac.ScopedRoleOrgAdmin(defaultOrgID))
		got := authorizedSlugs(orgAdmin)
		require.Contains(t, got, enabledDefault.Slug)
		require.Contains(t, got, disabledDefault.Slug)
		require.NotContains(t, got, otherOrgConfig.Slug)
	})

	t.Run("OrgAuditorSeesOwnOrgOnly", func(t *testing.T) {
		_, orgAuditor := coderdtest.CreateAnotherUser(t, adminClient, defaultOrgID, rbac.ScopedRoleOrgAuditor(defaultOrgID))
		got := authorizedSlugs(orgAuditor)
		require.Contains(t, got, enabledDefault.Slug)
		require.Contains(t, got, disabledDefault.Slug)
		require.NotContains(t, got, otherOrgConfig.Slug)
	})

	t.Run("OrgMemberSeesOwnOrgOnly", func(t *testing.T) {
		// A plain member of the default org, relying on the temporary
		// orgMember read grant.
		memberClient, member := coderdtest.CreateAnotherUser(t, adminClient, defaultOrgID)
		_ = memberClient
		got := authorizedSlugs(member)
		require.Contains(t, got, enabledDefault.Slug)
		require.Contains(t, got, disabledDefault.Slug)
		require.NotContains(t, got, otherOrgConfig.Slug)
	})

	t.Run("CustomSiteReadRoleSeesAcrossOrgs", func(t *testing.T) {
		// A site custom role holding only mcp_server_config:read. Custom
		// roles persisted through dbauthz cannot carry site permissions,
		// so write it directly to the raw database. The user lives in the
		// second org so no implicit organization read on the default org
		// masks a broken authorization path.
		customRole, err := database.New(rawSQLDB).InsertCustomRole(ctx, database.InsertCustomRoleParams{
			Name: "mcp-config-reader",
			SitePermissions: []database.CustomRolePermission{
				{ResourceType: rbac.ResourceMCPServerConfig.Type, Action: policy.ActionRead},
			},
			OrgPermissions:  []database.CustomRolePermission{},
			UserPermissions: []database.CustomRolePermission{},
		})
		require.NoError(t, err)
		user := dbgen.User(t, db, database.User{RBACRoles: []string{customRole.Name}})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: secondOrg.ID,
		})

		subject := mcpCustomRoleSubject(ctx, t, db, user)
		// The custom site role grants read across orgs, so this user sees
		// both orgs' configs.
		cfgs, err := authzDB().GetMCPServerConfigs(dbauthz.As(ctx, subject))
		require.NoError(t, err)
		got := make([]string, 0, len(cfgs))
		for _, c := range cfgs {
			got = append(got, c.Slug)
		}
		require.Contains(t, got, enabledDefault.Slug)
		require.Contains(t, got, otherOrgConfig.Slug)
	})
}

// mcpCustomRoleSubject builds the authorization subject for a dbgen user
// with custom RBAC roles, expanding roles and org membership from the DB.
func mcpCustomRoleSubject(ctx context.Context, t *testing.T, db database.Store, user database.User) rbac.Subject {
	t.Helper()

	roles := rbac.RoleIdentifiers{rbac.RoleMember()}
	for _, name := range user.RBACRoles {
		roles = append(roles, rbac.RoleIdentifier{Name: name})
	}
	orgs, err := db.GetOrganizationsByUserID(dbauthz.AsSystemRestricted(ctx), database.GetOrganizationsByUserIDParams{
		UserID:  user.ID,
		Deleted: sql.NullBool{Valid: true, Bool: false},
	})
	require.NoError(t, err)
	for _, org := range orgs {
		roles = append(roles, rbac.ScopedRoleOrgMember(org.ID))
	}
	rbacRoles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, roles)
	require.NoError(t, err)

	return rbac.Subject{
		ID:     user.ID.String(),
		Roles:  rbacRoles,
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	}.WithCachedASTValue()
}

// TestMCPServerConfigDeploymentConfigOnlyRoleWritesThroughWindow proves the
// interim write gate stays on deployment_config while the B1 fallback window
// is open: a site custom role holding only deployment_config read+update can
// create, update, and delete an MCP server config. Swapping the write-side
// dbauthz checks (or the concealed-404 fetch in the update/delete flows) to
// the new resource early would contract this write set, a behavior
// regression (invariant 1).
func TestMCPServerConfigDeploymentConfigOnlyRoleWritesThroughWindow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawStore, _, rawSQLDB := dbtestutil.NewDBWithSQLDB(t)
	client := coderdtest.New(t, &coderdtest.Options{Database: rawStore})
	_ = coderdtest.CreateFirstUser(t, client)
	db := rawStore

	// The user belongs to a non-default org and holds only deployment_config
	// read+update via a site custom role written directly to the raw
	// database (dbauthz cannot persist site permissions on custom roles).
	secondOrg := dbgen.Organization(t, db, database.Organization{})
	customRole, err := database.New(rawSQLDB).InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name: "deployment-config-manager",
		SitePermissions: []database.CustomRolePermission{
			{ResourceType: rbac.ResourceDeploymentConfig.Type, Action: policy.ActionRead},
			{ResourceType: rbac.ResourceDeploymentConfig.Type, Action: policy.ActionUpdate},
		},
		OrgPermissions:  []database.CustomRolePermission{},
		UserPermissions: []database.CustomRolePermission{},
	})
	require.NoError(t, err)
	user := dbgen.User(t, db, database.User{RBACRoles: []string{customRole.Name}})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: secondOrg.ID,
	})
	_, token := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	userClient := codersdk.New(client.URL)
	userClient.SetSessionToken(token)

	created, err := userClient.CreateMCPServerConfig(ctx, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "Window Server",
		Slug:          "window-server",
		Transport:     "streamable_http",
		URL:           "https://mcp.example.com/window",
		AuthType:      "none",
		Availability:  "default_on",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.NoError(t, err)

	newName := "Window Server Renamed"
	_, err = userClient.UpdateMCPServerConfig(ctx, created.ID, codersdk.UpdateMCPServerConfigRequest{
		DisplayName: &newName,
	})
	require.NoError(t, err)

	require.NoError(t, userClient.DeleteMCPServerConfig(ctx, created.ID))
}

// TestMCPServerConfigManagementListParentEquivalence proves the interim
// privileged management list keeps the parent's read contract: any principal
// the deployment_config read gate admits sees the full unfiltered row set
// (default-org enabled AND disabled, and any other org's rows), exactly as on
// the parent. The authorized-list swap must not narrow this path during the
// window. The subject holds ONLY deployment_config read via a site custom
// role and is a member of a non-default org, so the implicit default-org
// member read cannot mask a broken authorization path.
func TestMCPServerConfigManagementListParentEquivalence(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawStore, _, rawSQLDB := dbtestutil.NewDBWithSQLDB(t)
	client := coderdtest.New(t, &coderdtest.Options{Database: rawStore})
	_ = coderdtest.CreateFirstUser(t, client)
	db := rawStore

	// Seed default-org enabled + disabled configs (via the admin HTTP path)
	// and a config in the subject's own (second) org.
	enabledDefault := createMCPServerConfig(t, client, "enabled-default", true)
	disabledDefault := createMCPServerConfig(t, client, "disabled-default", false)
	secondOrg := dbgen.Organization(t, db, database.Organization{})
	ownOrgConfig := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: secondOrg.ID,
		Enabled:        true,
	})

	// A site custom role holding ONLY deployment_config read, member of the
	// non-default second org.
	customRole, err := database.New(rawSQLDB).InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name: "deployment-config-reader",
		SitePermissions: []database.CustomRolePermission{
			{ResourceType: rbac.ResourceDeploymentConfig.Type, Action: policy.ActionRead},
		},
		OrgPermissions:  []database.CustomRolePermission{},
		UserPermissions: []database.CustomRolePermission{},
	})
	require.NoError(t, err)
	user := dbgen.User(t, db, database.User{RBACRoles: []string{customRole.Name}})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: secondOrg.ID,
	})
	_, token := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})
	userClient := codersdk.New(client.URL)
	userClient.SetSessionToken(token)

	cfgs, err := userClient.MCPServerConfigs(ctx)
	require.NoError(t, err)
	got := make(map[string]bool, len(cfgs))
	for _, c := range cfgs {
		got[c.Slug] = true
	}
	require.True(t, got[enabledDefault.Slug], "privileged list missing default-org enabled config")
	require.True(t, got[disabledDefault.Slug], "privileged list missing default-org disabled config")
	require.True(t, got[ownOrgConfig.Slug], "privileged list missing subject-org config")

	// The privileged view carries connection metadata, not just redaction.
	var enabled codersdk.MCPServerConfig
	found := false
	for _, c := range cfgs {
		if c.Slug == enabledDefault.Slug {
			enabled = c
			found = true
		}
	}
	require.True(t, found)
	require.Equal(t, "https://mcp.example.com/"+enabledDefault.Slug, enabled.URL)
}
