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
// role after chat model config reads become organization-scoped.
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
	disabledProvider := dbgen.AIProvider(t, rawDB, database.AIProvider{
		Type: database.AIProviderTypeAnthropic,
	}, func(params *database.InsertAIProviderParams) {
		params.Enabled = false
	})
	providerDisabled := dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{
		AIProviderID:   uuid.NullUUID{UUID: disabledProvider.ID, Valid: true},
		OrganizationID: defaultOrg.ID,
		GroupACL: database.ChatACL{
			defaultOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
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
	require.False(t, disabledProvider.Enabled)
	require.True(t, providerDisabled.Enabled)
	require.True(t, denied.Enabled)
	require.True(t, otherEnabled.Enabled)

	for _, testCase := range []struct {
		name                 string
		scopes               []codersdk.APIKeyScope
		wantCollectionStatus int
		wantItemStatus       int
		wantACLStatus        int
	}{
		{
			name: "ModelReadWithOrganizationRead",
			scopes: []codersdk.APIKeyScope{
				codersdk.APIKeyScopeOrganizationRead,
				codersdk.APIKeyScopeChatModelConfigRead,
			},
			wantACLStatus: http.StatusNotFound,
		},
		{
			name:                 "WorkspaceRead",
			scopes:               []codersdk.APIKeyScope{codersdk.APIKeyScopeOrganizationRead, codersdk.APIKeyScopeWorkspaceRead},
			wantCollectionStatus: http.StatusForbidden,
			wantItemStatus:       http.StatusNotFound,
			wantACLStatus:        http.StatusNotFound,
		},
	} {
		t.Run("TokenScope/"+testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			token, err := client.CreateToken(ctx, codersdk.Me, codersdk.CreateTokenRequest{
				Scopes: testCase.scopes,
			})
			require.NoError(t, err)
			scopedClient := codersdk.New(
				client.URL,
				codersdk.WithSessionToken(token.Key),
				codersdk.WithHTTPClient(coderdtest.NewIsolatedHTTPClient(client.URL)),
			)
			t.Cleanup(scopedClient.HTTPClient.CloseIdleConnections)

			experimentalClient := codersdk.NewExperimentalClient(scopedClient)
			response, listErr := experimentalClient.ChatModels(ctx, defaultOrg.ID)
			model, itemErr := experimentalClient.ChatModel(ctx, defaultOrg.ID, ownEnabled.ID)
			_, aclErr := experimentalClient.ChatModelACL(ctx, defaultOrg.ID, ownEnabled.ID)
			if testCase.wantCollectionStatus != 0 {
				requireSDKError(t, listErr, testCase.wantCollectionStatus)
			} else {
				require.NoError(t, listErr)
				require.NotEmpty(t, response.Models)
			}
			if testCase.wantItemStatus != 0 {
				requireSDKError(t, itemErr, testCase.wantItemStatus)
			} else {
				require.NoError(t, itemErr)
				require.Equal(t, ownEnabled.ID, model.ID)
			}
			requireSDKError(t, aclErr, testCase.wantACLStatus)
		})
	}

	memberClientRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	for _, endpoint := range []struct {
		name string
		call func(context.Context, uuid.UUID) error
	}{
		{
			name: "Collection",
			call: func(ctx context.Context, organizationID uuid.UUID) error {
				_, err := memberClient.ChatModels(ctx, organizationID)
				return err
			},
		},
		{
			name: "Item",
			call: func(ctx context.Context, organizationID uuid.UUID) error {
				_, err := memberClient.ChatModel(ctx, organizationID, otherEnabled.ID)
				return err
			},
		},
		{
			name: "ACL",
			call: func(ctx context.Context, organizationID uuid.UUID) error {
				_, err := memberClient.ChatModelACL(ctx, organizationID, otherEnabled.ID)
				return err
			},
		},
	} {
		t.Run("ConcealsHiddenOrganization/"+endpoint.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			var concealedMessage string
			for _, organizationID := range []uuid.UUID{otherOrg.ID, uuid.New()} {
				err := endpoint.call(ctx, organizationID)
				sdkErr := requireSDKError(t, err, http.StatusNotFound)
				if concealedMessage == "" {
					concealedMessage = sdkErr.Message
				}
				require.Equal(t, concealedMessage, sdkErr.Message)
			}
		})
	}

	testCases := []struct {
		name       string
		client     func(t *testing.T, ctx context.Context) *codersdk.ExperimentalClient
		seesDenied bool
	}{
		{
			name: "AgentsAccess",
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(
					t,
					client.Client,
					defaultOrg.ID,
					rbac.ScopedRoleAgentsAccess(defaultOrg.ID),
				)
				return codersdk.NewExperimentalClient(rawClient)
			},
		},
		{
			name:       "Owner",
			seesDenied: true,
			client: func(*testing.T, context.Context) *codersdk.ExperimentalClient {
				return client
			},
		},
		{
			name:       "SiteAuditor",
			seesDenied: true,
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.RoleAuditor())
				return codersdk.NewExperimentalClient(rawClient)
			},
		},
		{
			name:       "CustomSiteReadRole",
			seesDenied: true,
			client: func(t *testing.T, ctx context.Context) *codersdk.ExperimentalClient {
				return newSiteCustomRoleClient(ctx, t, client, rawDB, defaultOrg.ID, database.CustomRolePermission{
					ResourceType: rbac.ResourceChatModelConfig.Type,
					Action:       policy.ActionRead,
				})
			},
		},
		{
			name:       "OrgAdmin",
			seesDenied: true,
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAdmin(defaultOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
		},
		{
			name:       "OrgAuditor",
			seesDenied: true,
			client: func(t *testing.T, _ context.Context) *codersdk.ExperimentalClient {
				rawClient, _ := coderdtest.CreateAnotherUser(t, client.Client, defaultOrg.ID, rbac.ScopedRoleOrgAuditor(defaultOrg.ID))
				return codersdk.NewExperimentalClient(rawClient)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			models, err := testCase.client(t, ctx).ChatModels(ctx, defaultOrg.ID)
			require.NoError(t, err)
			require.True(t, containsChatModel(models.Models, ownEnabled.ID))
			require.True(t, containsChatModel(models.Models, ownDisabled.ID))
			require.True(t, containsChatModel(models.Models, providerDisabled.ID))
			require.Equal(t, testCase.seesDenied, containsChatModel(models.Models, denied.ID))
			require.False(t, containsChatModel(models.Models, otherEnabled.ID))

			disabledProviderIndex := slices.IndexFunc(models.Providers, func(provider codersdk.ChatModelProviderDescriptor) bool {
				return provider.ID == disabledProvider.ID
			})
			require.NotEqual(t, -1, disabledProviderIndex)
			require.False(t, models.Providers[disabledProviderIndex].Enabled)
			require.False(t, models.Providers[disabledProviderIndex].Available)
		})
	}
}

func containsChatModel(models []codersdk.ChatModel, id uuid.UUID) bool {
	return slices.ContainsFunc(models, func(model codersdk.ChatModel) bool {
		return model.ID == id
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
