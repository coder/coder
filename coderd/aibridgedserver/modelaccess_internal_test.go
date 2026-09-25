package aibridgedserver

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

func TestAuthorizeModelUse(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	orgID := uuid.New()
	modelID := uuid.New()
	forbiddenErr := xerrors.New("forbidden model configuration lookup")

	cases := []struct {
		name        string
		roles       []string
		status      database.UserStatus
		configs     []database.GetAIModelAccessConfigsRow
		queryErr    error
		wantAllowed bool
		wantQuery   bool
		wantError   error
	}{
		{name: "nil authorizer fails closed", wantError: xerrors.New("model authorizer is not configured")},
		{name: "owner allows every model without lookup", roles: []string{rbac.RoleOwner().String()}, wantAllowed: true},
		{name: "site unrestricted role allows every model without lookup", roles: []string{rbac.RoleAIGatewayUnrestricted().String()}, wantAllowed: true},
		{name: "site user admin allows every model without lookup", roles: []string{rbac.RoleUserAdmin().String()}, wantAllowed: true},
		{name: "organization admin cannot use unconfigured models", roles: []string{rbac.ScopedRoleOrgAdmin(orgID).String()}, wantQuery: true},
		{name: "nonmember cannot use configured models", roles: []string{rbac.RoleMember().String()}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, wantQuery: true},
		{name: "configuration in another organization is denied", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: uuid.New()}}, wantQuery: true},
		{name: "later authorized configuration grants access", roles: []string{orgMemberRole(orgID)}, configs: []database.GetAIModelAccessConfigsRow{{ID: uuid.New(), OrganizationID: uuid.New()}, {ID: modelID, OrganizationID: orgID}}, wantAllowed: true, wantQuery: true},
		{name: "inactive user is denied", roles: []string{rbac.RoleOwner().String()}, status: database.UserStatusSuspended},
		{name: "forbidden configuration lookup is evaluation error", roles: []string{orgMemberRole(orgID)}, queryErr: dbauthz.NotAuthorizedError{Err: forbiddenErr}, wantQuery: true, wantError: forbiddenErr},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := dbmock.NewMockStore(gomock.NewController(t))
			key := database.APIKey{
				UserID: userID, Scopes: database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				AllowList: database.AllowList{{Type: "*", ID: "*"}},
			}
			if tc.name == "nil authorizer fails closed" {
				srv, err := NewServer(t.Context(), Options{Store: store, Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})})
				require.NoError(t, err)
				allowed, err := srv.authorizeModelUse(t.Context(), key, "provider", "model")
				require.EqualError(t, err, tc.wantError.Error())
				require.False(t, allowed)
				return
			}
			status := tc.status
			if status == "" {
				status = database.UserStatusActive
			}
			store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), userID).Return(database.GetAuthorizationUserRolesRow{ID: userID, Username: "model-user", Status: status, Roles: tc.roles}, nil)
			store.EXPECT().CustomRoles(gomock.Any(), gomock.Any()).AnyTimes().Return(customModelRoles(orgID, tc.roles), nil)

			if tc.wantQuery {
				store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{UserID: userID, ProviderName: "provider", Model: "model"}).Return(tc.configs, tc.queryErr)
			}
			srv, err := NewServer(t.Context(), Options{Store: store, Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()), Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})})
			require.NoError(t, err)
			allowed, err := srv.authorizeModelUse(t.Context(), key, "provider", "model")
			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantAllowed, allowed)
		})
	}
}

func orgMemberRole(orgID uuid.UUID) string {
	return (rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: orgID}).String()
}

func customModelRoles(orgID uuid.UUID, roleNames []string) []database.CustomRole {
	roles := make([]database.CustomRole, 0, 1)
	if slices.Contains(roleNames, orgMemberRole(orgID)) {
		perms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
		roles = append(roles, database.CustomRole{
			Name:              rbac.RoleOrgMember(),
			OrganizationID:    uuid.NullUUID{UUID: orgID, Valid: true},
			IsSystem:          true,
			OrgPermissions:    rolestore.ConvertPermissionsToDB(perms.Org),
			MemberPermissions: rolestore.ConvertPermissionsToDB(perms.Member),
		})
	}
	return roles
}

type modelAuthorizer struct {
	rbac.Authorizer
	authorize func(rbac.Subject, policy.Action, rbac.Object) error
}

func (a modelAuthorizer) Authorize(_ context.Context, subject rbac.Subject, action policy.Action, obj rbac.Object) error {
	return a.authorize(subject, action, obj)
}

func TestAuthorizeModelUseErrors(t *testing.T) {
	t.Parallel()

	for _, failure := range []error{xerrors.New("evaluation failed"), context.Canceled} {
		for _, stage := range []string{"Subject", "Unrestricted", "Load", "Configured"} {
			t.Run(failure.Error()+"/"+stage, func(t *testing.T) {
				t.Parallel()
				store := dbmock.NewMockStore(gomock.NewController(t))
				key := database.APIKey{
					UserID: uuid.New(), Scopes: database.APIKeyScopes{database.ApiKeyScopeChatModelConfigUse},
					AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}},
				}
				config := database.GetAIModelAccessConfigsRow{ID: uuid.New(), OrganizationID: uuid.New()}
				var subjectErr error
				if stage == "Subject" {
					subjectErr = failure
				}
				store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), key.UserID).Return(database.GetAuthorizationUserRolesRow{
					ID: key.UserID, Status: database.UserStatusActive, Roles: []string{rbac.RoleMember().String()},
				}, subjectErr)
				if stage == "Load" || stage == "Configured" {
					var queryErr error
					if stage == "Load" {
						queryErr = failure
					}
					store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{
						UserID: key.UserID, ProviderName: "provider", Model: "model",
					}).Return([]database.GetAIModelAccessConfigsRow{config, {ID: uuid.New(), OrganizationID: uuid.New()}}, queryErr)
				}

				var objects []rbac.Object
				auth := modelAuthorizer{authorize: func(subject rbac.Subject, action policy.Action, obj rbac.Object) error {
					require.Equal(t, key.UserID.String(), subject.ID)
					require.Equal(t, key.ScopeSet(), subject.Scope)
					require.Equal(t, policy.ActionUse, action)
					objects = append(objects, obj)
					if stage == "Unrestricted" || obj.Type == rbac.ResourceChatModelConfig.Type {
						return failure
					}
					return &rbac.UnauthorizedError{}
				}}
				srv := &Server{store: store, authorizer: auth}
				allowed, err := srv.authorizeModelUse(t.Context(), key, "provider", "model")
				require.ErrorIs(t, err, failure)
				require.False(t, allowed)
				switch stage {
				case "Subject":
					require.Empty(t, objects)
				case "Configured":
					require.Equal(t, []rbac.Object{
						rbac.ResourceAIGatewayUnrestricted,
						rbac.ResourceChatModelConfig.WithID(config.ID).InOrg(config.OrganizationID),
					}, objects, "evaluation errors must stop before considering another grant")
				default:
					require.Equal(t, []rbac.Object{rbac.ResourceAIGatewayUnrestricted}, objects)
				}
			})
		}
	}
}

func TestAuthorizeModelUseDatabase(t *testing.T) {
	t.Parallel()

	rawDB, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	authzDB := dbauthz.New(rawDB, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), logger, nil)
	srv, err := NewServer(t.Context(), Options{Store: authzDB, Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()), Logger: logger})
	require.NoError(t, err)

	org := dbgen.Organization(t, rawDB, database.Organization{})
	provider := dbgen.AIProvider(t, rawDB, database.AIProvider{})
	dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: org.ID, Model: "configured", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})
	user := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: user.ID})

	key, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: user.ID})
	access := func(key database.APIKey, model string) bool {
		allowed, err := srv.authorizeModelUse(t.Context(), key, provider.Name, model)
		require.NoError(t, err)
		return allowed
	}

	require.False(t, access(key, "unknown"), "new org members cannot use unconfigured models")
	require.True(t, access(key, "configured"), "org members may use configured models")

	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: user.ID, GrantedRoles: []string{rbac.RoleAIGatewayUnrestricted().Name}})
	require.NoError(t, err)
	require.True(t, access(key, "unknown"), "explicit site role grants unrestricted use")
	require.NoError(t, rawDB.DeleteOrganizationMember(t.Context(), database.DeleteOrganizationMemberParams{OrganizationID: org.ID, UserID: user.ID}))
	require.True(t, access(key, "unknown"), "site grants do not depend on organization membership")
	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: user.ID, GrantedRoles: []string{}})
	require.NoError(t, err)
	require.False(t, access(key, "unknown"), "removing the site role revokes unrestricted use")
	require.False(t, access(key, "configured"), "configured use requires membership without a site grant")

	deletedOrg := dbgen.Organization(t, rawDB, database.Organization{})
	dbgen.ChatModelConfig(t, rawDB, database.ChatModelConfig{OrganizationID: deletedOrg.ID, Model: "configured", AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: deletedOrg.ID, UserID: user.ID})
	require.True(t, access(key, "configured"))
	require.NoError(t, rawDB.UpdateOrganizationDeletedByID(t.Context(), database.UpdateOrganizationDeletedByIDParams{ID: deletedOrg.ID, UpdatedAt: dbtime.Now()}))
	require.False(t, access(key, "configured"), "retained membership in a deleted organization cannot grant configured use")

	serviceAccount := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, IsServiceAccount: true, RBACRoles: []string{rbac.RoleAIGatewayUnrestricted().Name}})
	dbgen.OrganizationMember(t, rawDB, database.OrganizationMember{OrganizationID: org.ID, UserID: serviceAccount.ID})
	serviceKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: serviceAccount.ID})
	require.True(t, access(serviceKey, "unknown"), "service accounts can receive explicit site grants")
	_, err = rawDB.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: serviceAccount.ID, GrantedRoles: []string{}})
	require.NoError(t, err)
	require.False(t, access(serviceKey, "unknown"))
	require.True(t, access(serviceKey, "configured"), "service accounts retain configured use after site grant revocation")

	owner := dbgen.User(t, rawDB, database.User{Status: database.UserStatusActive, RBACRoles: []string{rbac.RoleOwner().Name}})
	scopedKey, _ := dbgen.APIKey(t, rawDB, database.APIKey{UserID: owner.ID, AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}})
	require.False(t, access(scopedKey, "configured"), "owner access is constrained by key scope allow-list")
}
