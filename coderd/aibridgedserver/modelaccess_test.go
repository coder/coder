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
		{name: "organization unrestricted role allows every model", roles: []string{rbac.ScopedRoleOrgAIGatewayUnrestricted(orgID).String()}, wantAllowed: true, wantReason: aibridgedserver.ModelAccessAllowed},
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

	org := dbgen.Organization(t, rawDB, database.Organization{DefaultOrgMemberRoles: []string{rbac.RoleOrgAIGatewayUnrestricted()}})
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

	require.True(t, access(key, "unknown").Allowed, "default org role grants unrestricted use")
	_, err = rawDB.UpdateOrganization(t.Context(), database.UpdateOrganizationParams{ID: org.ID, UpdatedAt: dbtime.Now(), Name: org.Name, DisplayName: org.DisplayName, Description: org.Description, Icon: org.Icon, DefaultOrgMemberRoles: []string{}})
	require.NoError(t, err)
	require.False(t, access(key, "unknown").Allowed, "removing default role revokes unrestricted use")
	require.True(t, access(key, "configured").Allowed, "org member may still use configured models")

	_, err = rawDB.UpdateMemberRoles(t.Context(), database.UpdateMemberRolesParams{UserID: user.ID, OrgID: org.ID, GrantedRoles: []string{rbac.RoleOrgAIGatewayUnrestricted()}})
	require.NoError(t, err)
	require.True(t, access(key, "unknown").Allowed, "explicit org role grants unrestricted use")
	_, err = rawDB.UpdateMemberRoles(t.Context(), database.UpdateMemberRolesParams{UserID: user.ID, OrgID: org.ID})
	require.NoError(t, err)
	require.False(t, access(key, "unknown").Allowed, "removing explicit org role revokes use")

	require.NoError(t, rawDB.DeleteOrganizationMember(t.Context(), database.DeleteOrganizationMemberParams{OrganizationID: org.ID, UserID: user.ID}))
	require.False(t, access(key, "configured").Allowed, "removing membership revokes configured use")
	deletedOrg := dbgen.Organization(t, rawDB, database.Organization{DefaultOrgMemberRoles: []string{rbac.RoleOrgAIGatewayUnrestricted()}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: deletedOrg.ID, UserID: user.ID})
	require.True(t, access(key, "unknown").Allowed)
	require.NoError(t, rawDB.UpdateOrganizationDeletedByID(t.Context(), database.UpdateOrganizationDeletedByIDParams{ID: deletedOrg.ID, UpdatedAt: dbtime.Now()}))
	require.False(t, access(key, "unknown").Allowed, "retained membership in a deleted organization cannot grant unrestricted use")

	serviceAccount := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, IsServiceAccount: true})
	serviceOrg := dbgen.Organization(t, rawDB, database.Organization{DefaultOrgMemberRoles: []string{rbac.RoleOrgAIGatewayUnrestricted()}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: serviceOrg.ID, UserID: serviceAccount.ID})
	serviceKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: serviceAccount.ID})
	require.True(t, access(serviceKey, "unknown").Allowed, "service account inherits default org role")

	owner := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, RBACRoles: []string{rbac.RoleOwner().Name}})
	scopedKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: owner.ID, AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}})
	require.False(t, access(scopedKey, "configured").Allowed, "owner access is constrained by key scope allow-list")
}
