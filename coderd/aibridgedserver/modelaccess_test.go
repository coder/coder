package aibridgedserver_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
)

func TestResolveModelAccess(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgID := uuid.New()
	modelID := uuid.New()
	forbiddenErr := xerrors.New("forbidden model configuration lookup")
	queryErr := xerrors.New("database unavailable")

	cases := []struct {
		name        string
		roles       []string
		status      database.UserStatus
		configs     []database.GetAIModelAccessConfigsRow
		queryErr    error
		allowList   database.AllowList
		wantAllowed bool
		wantReason  aibridgedserver.ModelAccessReason
		wantError   error
	}{
		{name: "nil authorizer fails closed", wantReason: aibridgedserver.ModelAccessError, wantError: xerrors.New("model authorizer is not configured")},
		{name: "owner allows every model", roles: []string{rbac.RoleOwner().String()}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "site unrestricted role allows every model", roles: []string{rbac.RoleAIGatewayUnrestricted().String()}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "site user admin allows every model", roles: []string{rbac.RoleUserAdmin().String()}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "organization admin cannot use unconfigured models", roles: []string{rbac.ScopedRoleOrgAdmin(orgID).String()}, wantReason: aibridgedserver.ModelAccessDenied},
		{name: "organization admin can use configured models", roles: []string{rbac.ScopedRoleOrgAdmin(orgID).String()}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "configured model is allowed by organization permission", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "missing configured model is denied", roles: []string{orgMemberRole(orgID)}, wantReason: aibridgedserver.ModelAccessDenied},
		{name: "model ACL does not grant access", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "scope allow list denies unrelated model", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}, wantReason: aibridgedserver.ModelAccessDenied},
		{name: "scope allow list permits configured model", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: modelID.String()}}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
		{name: "inactive user is denied", roles: []string{rbac.RoleOwner().String()}, status: database.UserStatusSuspended, wantReason: aibridgedserver.ModelAccessDenied},
		{name: "configuration query failure is evaluation error", roles: []string{orgMemberRole(orgID)}, queryErr: queryErr, wantReason: aibridgedserver.ModelAccessError, wantError: queryErr},
		{name: "forbidden configuration lookup is evaluation error", roles: []string{orgMemberRole(orgID)}, queryErr: dbauthz.NotAuthorizedError{Err: forbiddenErr}, wantReason: aibridgedserver.ModelAccessError, wantError: forbiddenErr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := dbmock.NewMockStore(gomock.NewController(t))
			allowList := tc.allowList
			if len(allowList) == 0 {
				allowList = database.AllowList{{Type: "*", ID: "*"}}
			}
			key := database.APIKey{UserID: userID, Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderAll}, AllowList: allowList}
			if tc.name == "nil authorizer fails closed" {
				srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: store, Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})})
				require.NoError(t, err)
				result, err := srv.ResolveModelAccess(t.Context(), key, "provider", "model")
				require.EqualError(t, err, tc.wantError.Error())
				require.False(t, result.Allowed)
				require.Equal(t, tc.wantReason, result.Reason)
				return
			}
			status := tc.status
			if status == "" {
				status = database.UserStatusActive
			}
			store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), userID).Return(database.GetAuthorizationUserRolesRow{ID: userID, Username: "model-user", Status: status, Roles: tc.roles}, nil)
			store.EXPECT().CustomRoles(gomock.Any(), gomock.Any()).AnyTimes().Return(customModelRoles(orgID, tc.roles), nil)

			store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{UserID: userID, ProviderName: "provider", Model: "model"}).Return(tc.configs, tc.queryErr).AnyTimes()
			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: store, Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()), Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})})
			require.NoError(t, err)
			result, err := srv.ResolveModelAccess(t.Context(), key, "provider", "model")
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantAllowed, result.Allowed)
			require.Equal(t, tc.wantReason, result.Reason)
		})
	}
}

func orgMemberRole(orgID uuid.UUID) string {
	return (rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: orgID}).String()
}

func customRolePermissions(perms []rbac.Permission) database.CustomRolePermissions {
	out := make(database.CustomRolePermissions, 0, len(perms))
	for _, perm := range perms {
		out = append(out, database.CustomRolePermission{Negate: perm.Negate, ResourceType: perm.ResourceType, Action: perm.Action})
	}
	return out
}

func customModelRoles(orgID uuid.UUID, roleNames []string) []database.CustomRole {
	roles := make([]database.CustomRole, 0, 1)
	if slices.Contains(roleNames, orgMemberRole(orgID)) {
		perms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
		roles = append(roles, database.CustomRole{
			Name:              rbac.RoleOrgMember(),
			OrganizationID:    uuid.NullUUID{UUID: orgID, Valid: true},
			IsSystem:          true,
			OrgPermissions:    customRolePermissions(perms.Org),
			MemberPermissions: customRolePermissions(perms.Member),
		})
	}
	return roles
}

func TestResolveModelAccessDatabase(t *testing.T) {
	t.Parallel()

	rawDB, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, coderdtest.AccessControlStorePointer())
	srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{Store: authzDB, Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()), Logger: logger})
	require.NoError(t, err)

	org := dbgen.Organization(t, rawDB, database.Organization{})
	provider := dbgen.AIProvider(t, rawDB, database.AIProvider{})
	dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: org.ID, Model: "configured", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})
	user := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})

	key, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: user.ID})
	access := func(key database.APIKey, model string) aibridgedserver.ModelAccess {
		result, err := srv.ResolveModelAccess(t.Context(), key, provider.Name, model)
		require.NoError(t, err)
		return result
	}

	require.False(t, access(key, "unknown").Allowed, "new org members cannot use unconfigured models")
	require.True(t, access(key, "configured").Allowed, "org members may use configured models")

	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: user.ID, GrantedRoles: []string{rbac.RoleAIGatewayUnrestricted().Name}})
	require.NoError(t, err)
	require.True(t, access(key, "unknown").Allowed, "explicit site role grants unrestricted use")
	require.NoError(t, rawDB.DeleteOrganizationMember(t.Context(), database.DeleteOrganizationMemberParams{OrganizationID: org.ID, UserID: user.ID}))
	require.True(t, access(key, "unknown").Allowed, "site grants do not depend on organization membership")
	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: user.ID, GrantedRoles: []string{}})
	require.NoError(t, err)
	require.False(t, access(key, "unknown").Allowed, "removing the site role revokes unrestricted use")
	require.False(t, access(key, "configured").Allowed, "configured use requires membership without a site grant")

	deletedOrg := dbgen.Organization(t, rawDB, database.Organization{})
	dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: deletedOrg.ID, Model: "configured", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: deletedOrg.ID, UserID: user.ID})
	require.True(t, access(key, "configured").Allowed)
	require.NoError(t, rawDB.UpdateOrganizationDeletedByID(t.Context(), database.UpdateOrganizationDeletedByIDParams{ID: deletedOrg.ID, UpdatedAt: dbtime.Now()}))
	require.False(t, access(key, "configured").Allowed, "retained membership in a deleted organization cannot grant configured use")

	serviceAccount := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, IsServiceAccount: true, RBACRoles: []string{rbac.RoleAIGatewayUnrestricted().Name}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: serviceAccount.ID})
	serviceKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: serviceAccount.ID})
	require.True(t, access(serviceKey, "unknown").Allowed, "service accounts can receive explicit site grants")
	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: serviceAccount.ID, GrantedRoles: []string{}})
	require.NoError(t, err)
	require.False(t, access(serviceKey, "unknown").Allowed)
	require.True(t, access(serviceKey, "configured").Allowed, "service accounts retain configured use after site grant revocation")

	owner := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, RBACRoles: []string{rbac.RoleOwner().Name}})
	scopedKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: owner.ID, AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}})
	require.False(t, access(scopedKey, "configured").Allowed, "owner access is constrained by key scope allow-list")
}
