package aibridgedserver_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/cryptorand"
)

func stringPtr(value string) *string { return &value }

func orgMemberRole(orgID uuid.UUID) string {
	return (rbac.RoleIdentifier{Name: rbac.RoleOrgMember(), OrganizationID: orgID}).String()
}

func customModelRoles(orgID uuid.UUID, roleNames []string) []database.CustomRole {
	if !slices.Contains(roleNames, orgMemberRole(orgID)) {
		return nil
	}
	perms := rbac.OrgMemberPermissions(rbac.OrgSettings{})
	return []database.CustomRole{{
		Name:              rbac.RoleOrgMember(),
		OrganizationID:    uuid.NullUUID{UUID: orgID, Valid: true},
		IsSystem:          true,
		OrgPermissions:    rolestore.ConvertPermissionsToDB(perms.Org),
		MemberPermissions: rolestore.ConvertPermissionsToDB(perms.Member),
	}}
}

func TestIsAuthorizedModelAuthorizationRespectsScope(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	modelID := uuid.New()
	for _, tc := range []struct {
		name        string
		role        string
		scope       database.APIKeyScope
		allowList   database.AllowList
		queryErr    error
		wantAllowed bool
		skipQuery   bool
	}{
		{name: "member read scope cannot use", role: orgMemberRole(orgID), scope: database.ApiKeyScopeChatModelConfigRead},
		{name: "owner read scope cannot use", role: rbac.RoleOwner().String(), scope: database.ApiKeyScopeChatModelConfigRead},
		{name: "unrestricted role read scope cannot use", role: rbac.RoleAIGatewayUnrestricted().String(), scope: database.ApiKeyScopeChatModelConfigRead},
		{name: "unrestricted use scope permits site grant", role: rbac.RoleAIGatewayUnrestricted().String(), scope: database.ApiKeyScopeAIGatewayUnrestrictedUse, wantAllowed: true, skipQuery: true},
		{name: "unrestricted use scope permits owner", role: rbac.RoleOwner().String(), scope: database.ApiKeyScopeAIGatewayUnrestrictedUse, wantAllowed: true, skipQuery: true},
		{name: "unrestricted use scope cannot grant member access", role: orgMemberRole(orgID), scope: database.ApiKeyScopeAIGatewayUnrestrictedUse},
		{name: "configured use scope permits member", role: orgMemberRole(orgID), scope: database.ApiKeyScopeChatModelConfigUse, wantAllowed: true},
		{name: "configured use scope permits owner", role: rbac.RoleOwner().String(), scope: database.ApiKeyScopeChatModelConfigUse, wantAllowed: true},
		{name: "member allow list denies unrelated model", role: orgMemberRole(orgID), scope: database.ApiKeyScopeCoderAll, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}},
		{name: "owner allow list denies unrelated model", role: rbac.RoleOwner().String(), scope: database.ApiKeyScopeCoderAll, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}}},
		{name: "member allow list permits configured model", role: orgMemberRole(orgID), scope: database.ApiKeyScopeChatModelConfigUse, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: modelID.String()}}, wantAllowed: true},
		{name: "owner allow list permits configured model", role: rbac.RoleOwner().String(), scope: database.ApiKeyScopeChatModelConfigUse, allowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: modelID.String()}}, wantAllowed: true},
		{name: "evaluation failure is coded", role: orgMemberRole(orgID), scope: database.ApiKeyScopeCoderAll, queryErr: xerrors.New("model authorization unavailable")},
	} {
		for _, mode := range []string{"FullKey", "DelegatedID"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				t.Parallel()

				store := dbmock.NewMockStore(gomock.NewController(t))
				keyID, err := cryptorand.String(10)
				require.NoError(t, err)
				secret, hashedSecret, err := apikey.GenerateSecret(22)
				require.NoError(t, err)
				key := database.APIKey{
					ID: keyID, UserID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour),
					HashedSecret: hashedSecret, LoginType: database.LoginTypeToken,
					Scopes: database.APIKeyScopes{tc.scope}, AllowList: tc.allowList,
				}
				if key.AllowList == nil {
					key.AllowList = database.AllowList{{Type: "*", ID: "*"}}
				}
				store.EXPECT().GetAPIKeyByID(gomock.Any(), key.ID).Return(key, nil)
				store.EXPECT().GetUserByID(gomock.Any(), key.UserID).Return(database.User{
					ID: key.UserID, Username: "model-user", Status: database.UserStatusActive,
				}, nil)
				store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), key.UserID).Return(database.GetAuthorizationUserRolesRow{
					ID: key.UserID, Username: "model-user", Status: database.UserStatusActive, Roles: []string{tc.role},
				}, nil)
				store.EXPECT().CustomRoles(gomock.Any(), gomock.Any()).AnyTimes().Return(customModelRoles(orgID, []string{tc.role}), nil)
				if !tc.skipQuery {
					store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{
						UserID: key.UserID, ProviderName: "provider", Model: "model",
					}).Return([]database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, tc.queryErr)
				}
				srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
					Store: store, Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()),
					Logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				})
				require.NoError(t, err)
				req := &proto.IsAuthorizedRequest{ProviderName: stringPtr("provider"), Model: stringPtr("model")}
				if mode == "FullKey" {
					req.Key = key.ID + "-" + secret
				} else {
					req.KeyId = key.ID
				}
				resp, err := srv.IsAuthorized(t.Context(), req)
				if !tc.wantAllowed {
					require.Error(t, err)
					if tc.queryErr != nil {
						require.Equal(t, proto.AuthorizationErrorEvaluation, drpcerr.Code(err))
					} else {
						require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
					}
					require.Nil(t, resp)
					return
				}
				require.NoError(t, err)
				require.Equal(t, &proto.IsAuthorizedResponse{
					OwnerId: key.UserID.String(), ApiKeyId: key.ID, Username: "model-user",
				}, resp)
			})
		}
	}
}
