package coderd_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatModelConfigListReadContracts pins the visible config set for each
// role. The legacy endpoint reads only the default organization's configs.
func TestChatModelConfigListReadContracts(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, pubsub := dbtestutil.NewDB(t)
	client := newChatClient(t, func(opts *coderdtest.Options) {
		opts.Database = rawDB
		opts.Pubsub = pubsub
	})
	_ = coderdtest.CreateFirstUser(t, client.Client)

	defaultOrg, err := rawDB.GetDefaultOrganization(ctx)
	require.NoError(t, err)
	otherOrg := dbgen.Organization(t, rawDB, database.Organization{IsDefault: false})
	seedEveryoneGroup(t, rawDB, otherOrg.ID)

	ownEnabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: defaultOrg.ID,
		GroupACL: database.ChatACL{
			defaultOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})
	ownDisabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: defaultOrg.ID,
		GroupACL: database.ChatACL{
			defaultOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	}, func(params *database.InsertChatModelConfigParams) {
		params.Enabled = false
	})
	denied := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: defaultOrg.ID,
		GroupACL:       database.ChatACL{},
		UserACL:        database.ChatACL{},
	})
	otherEnabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		OrganizationID: otherOrg.ID,
		GroupACL: database.ChatACL{
			otherOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})
	require.True(t, ownEnabled.Enabled)
	require.False(t, ownDisabled.Enabled)
	require.True(t, denied.Enabled)
	require.True(t, otherEnabled.Enabled)

	for _, testCase := range []struct {
		name       string
		scope      codersdk.APIKeyScope
		wantStatus int
	}{
		{name: "ModelRead", scope: codersdk.APIKeyScopeChatModelConfigRead},
		{name: "WorkspaceRead", scope: codersdk.APIKeyScopeWorkspaceRead, wantStatus: http.StatusForbidden},
	} {
		t.Run("TokenScope/"+testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			token, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
				Scopes: []codersdk.APIKeyScope{testCase.scope},
			})
			require.NoError(t, err)
			scopedClient := codersdk.New(
				client.URL,
				codersdk.WithSessionToken(token.Key),
				codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(client.URL)),
			)
			t.Cleanup(scopedClient.HTTPClient.CloseIdleConnections)

			configs, err := codersdk.NewExperimentalClient(scopedClient).ListChatModelConfigs(ctx)
			if testCase.wantStatus != 0 {
				requireSDKError(t, err, testCase.wantStatus)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, configs)
		})
	}

	testCases := []struct {
		name    string
		client  func(t *testing.T, ctx context.Context) *codersdk.ExperimentalClient
		visible []uuid.UUID
		hidden  []uuid.UUID
	}{
		{
			name: "OwnerSeesDefaultOrgConfigs",
			client: func(*testing.T, context.Context) *codersdk.ExperimentalClient {
				return client
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID},
			hidden:  []uuid.UUID{otherEnabled.ID},
		},
		{
			name: "SiteAuditorSeesDefaultOrgConfigs",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.RoleAuditor())
				return codersdk.NewExperimentalClient(rawClient)
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID},
			hidden:  []uuid.UUID{otherEnabled.ID},
		},
		{
			name: "CustomSiteReadRoleSeesDefaultOrgConfigs",
			client: func(t *testing.T, ctx context.Context) *codersdk.ExperimentalClient {
				return newSiteCustomRoleClient(ctx, t, client, rawDB, defaultOrg.ID, database.CustomRolePermission{
					ResourceType: rbac.ResourceChatModelConfig.Type,
					Action:       policy.ActionRead,
				})
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID},
			hidden:  []uuid.UUID{otherEnabled.ID},
		},
		{
			name: "AgentsAccessUsesRowACL",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(
					t,
					client.Client,
					defaultOrg.ID,
					rbac.ScopedRoleAgentsAccess(defaultOrg.ID),
				)
				return codersdk.NewExperimentalClient(rawClient)
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID},
			hidden:  []uuid.UUID{denied.ID, otherEnabled.ID},
		},
		{
			name: "DeploymentConfigReadOnlyUsesRowACL",
			client: func(t *testing.T, ctx context.Context) *codersdk.ExperimentalClient {
				return newSiteCustomRoleClient(ctx, t, client, rawDB, defaultOrg.ID, database.CustomRolePermission{
					ResourceType: rbac.ResourceDeploymentConfig.Type,
					Action:       policy.ActionRead,
				})
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID},
			hidden:  []uuid.UUID{denied.ID, otherEnabled.ID},
		},
		{
			name: "DefaultOrgAdminKeepsEnabledList",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAdmin(defaultOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID},
			hidden:  []uuid.UUID{otherEnabled.ID},
		},
		{
			name: "DefaultOrgAuditorKeepsEnabledList",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAuditor(defaultOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
			visible: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID},
			hidden:  []uuid.UUID{otherEnabled.ID},
		},
		{
			name: "NonDefaultOrgAdminSeesNoDefaultOrgConfigs",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrg.ID, rbac.ScopedRoleOrgAdmin(otherOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
			hidden: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID, otherEnabled.ID},
		},
		{
			name: "NonDefaultOrgAuditorSeesNoDefaultOrgConfigs",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrg.ID, rbac.ScopedRoleOrgAuditor(otherOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
			hidden: []uuid.UUID{ownEnabled.ID, ownDisabled.ID, denied.ID, otherEnabled.ID},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			configs, err := testCase.client(t, ctx).ListChatModelConfigs(ctx)
			require.NoError(t, err)
			for _, id := range testCase.visible {
				require.True(t, containsChatModelConfig(configs, id), "must see config %s", id)
			}
			for _, id := range testCase.hidden {
				require.False(t, containsChatModelConfig(configs, id), "must not see config %s", id)
			}
		})
	}
}

func containsChatModelConfig(configs []codersdk.ChatModelConfig, id uuid.UUID) bool {
	return slices.ContainsFunc(configs, func(config codersdk.ChatModelConfig) bool {
		return config.ID == id
	})
}

func seedEveryoneGroup(t testing.TB, db database.Store, organizationID uuid.UUID) {
	t.Helper()
	dbgen.Group(t, db, database.Group{
		ID:             organizationID,
		Name:           database.EveryoneGroup,
		OrganizationID: organizationID,
	})
}

// newSiteCustomRoleClient seeds a null-org role through the raw store because
// public custom-role APIs create organization roles and reject site permissions.
func newSiteCustomRoleClient(
	ctx context.Context,
	t testing.TB,
	ownerClient *codersdk.ExperimentalClient,
	db database.Store,
	organizationID uuid.UUID,
	permissions ...database.CustomRolePermission,
) *codersdk.ExperimentalClient {
	t.Helper()

	role, err := db.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:            testutil.GetRandomName(t),
		DisplayName:     "Site Custom Test Role",
		OrganizationID:  uuid.NullUUID{},
		SitePermissions: permissions,
	})
	require.NoError(t, err)

	rawClient, user := coderdtest.CreateAnotherUser(t, ownerClient.Client, organizationID)
	_, err = ownerClient.UpdateUserRoles(ctx, user.ID.String(), codersdk.UpdateRoles{
		Roles: []string{role.Name},
	})
	require.NoError(t, err)
	return codersdk.NewExperimentalClient(rawClient)
}
