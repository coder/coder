package aibridgedserver_test

import (
	"database/sql"
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

func TestIsAuthorizedModelAuthorizationProtocol(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	keyID := "model-auth-key"
	orgID := uuid.New()
	modelID := uuid.New()
	queryErr := xerrors.New("model authorization unavailable")

	newKey := func() database.APIKey {
		return database.APIKey{
			ID:           keyID,
			UserID:       userID,
			ExpiresAt:    time.Now().Add(time.Hour),
			HashedSecret: []byte("unused-by-delegated-auth"),
			Scopes:       database.APIKeyScopes{database.ApiKeyScopeCoderAll},
			AllowList:    database.AllowList{{Type: "*", ID: "*"}},
		}
	}
	newUser := func() database.User {
		return database.User{
			ID:       userID,
			Username: "model-user",
			Status:   database.UserStatusActive,
		}
	}

	cases := []struct {
		name      string
		provider  *string
		model     *string
		roles     []string
		configs   []database.GetAIModelAccessConfigsRow
		queryErr  error
		keyErr    error
		wantCode  uint64
		wantErr   error
		wantResp  bool
		wantQuery bool
	}{
		{
			name:     "key only remains authentication only",
			wantResp: true,
		},
		{
			name:     "provider only is malformed",
			provider: stringPtr("provider"),
			wantCode: proto.AuthorizationErrorMalformed,
		},
		{
			name:     "model only is malformed",
			model:    stringPtr("model"),
			wantCode: proto.AuthorizationErrorMalformed,
		},
		{
			name:     "empty provider is malformed",
			provider: stringPtr(""),
			model:    stringPtr("model"),
			wantCode: proto.AuthorizationErrorMalformed,
		},
		{
			name:     "empty model is malformed",
			provider: stringPtr("provider"),
			model:    stringPtr(""),
			wantCode: proto.AuthorizationErrorMalformed,
		},
		{
			name:     "authentication failure is coded and comparable",
			keyErr:   sql.ErrNoRows,
			wantCode: proto.AuthorizationErrorAuthentication,
			wantErr:  aibridgedserver.ErrUnknownKey,
		},
		{
			name:      "policy denial is coded",
			provider:  stringPtr("provider"),
			model:     stringPtr("model"),
			roles:     []string{rbac.RoleMember().String()},
			configs:   []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}},
			wantCode:  proto.AuthorizationErrorPolicy,
			wantQuery: true,
		},
		{
			name:      "evaluation failure is coded",
			provider:  stringPtr("provider"),
			model:     stringPtr("model"),
			roles:     []string{orgMemberRole(orgID)},
			queryErr:  queryErr,
			wantCode:  proto.AuthorizationErrorEvaluation,
			wantQuery: true,
		},
		{
			name:      "configured model is authorized",
			provider:  stringPtr("provider"),
			model:     stringPtr("model"),
			roles:     []string{orgMemberRole(orgID)},
			configs:   []database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}},
			wantResp:  true,
			wantQuery: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := dbmock.NewMockStore(gomock.NewController(t))
			key := newKey()
			switch {
			case tc.keyErr != nil:
				store.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).Return(database.APIKey{}, tc.keyErr)
			case tc.provider == nil && tc.model == nil:
				store.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).Return(key, nil)
				store.EXPECT().GetUserByID(gomock.Any(), userID).Return(newUser(), nil)
			case tc.provider != nil && tc.model != nil && *tc.provider != "" && *tc.model != "":
				store.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).Return(key, nil)
				store.EXPECT().GetUserByID(gomock.Any(), userID).Return(newUser(), nil)
				store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), userID).Return(database.GetAuthorizationUserRolesRow{
					ID: userID, Username: "model-user", Status: database.UserStatusActive, Roles: tc.roles,
				}, nil)
				store.EXPECT().CustomRoles(gomock.Any(), gomock.Any()).AnyTimes().Return(customModelRoles(orgID, tc.roles), nil)
				if tc.wantQuery {
					store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{
						UserID: userID, ProviderName: *tc.provider, Model: *tc.model,
					}).Return(tc.configs, tc.queryErr)
				}
			}

			srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
				Store:      store,
				Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()),
				Logger:     slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			})
			require.NoError(t, err)

			req := &proto.IsAuthorizedRequest{KeyId: keyID}
			req.ProviderName = tc.provider
			req.Model = tc.model
			resp, err := srv.IsAuthorized(t.Context(), req)
			if tc.wantCode != 0 {
				require.Error(t, err)
				require.Equal(t, tc.wantCode, drpcerr.Code(err))
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				}
				require.Nil(t, resp)
				return
			}

			require.NoError(t, err)
			if tc.wantResp {
				require.Equal(t, &proto.IsAuthorizedResponse{
					OwnerId: userID.String(), ApiKeyId: keyID, Username: "model-user",
				}, resp)
			} else {
				require.Nil(t, resp)
			}
		})
	}
}

func TestIsAuthorizedModelAuthorizationUsesDelegatedIdentityOnly(t *testing.T) {
	t.Parallel()

	store := dbmock.NewMockStore(gomock.NewController(t))
	userID := uuid.New()
	keyID := "delegated-model-key"
	orgID := uuid.New()
	modelID := uuid.New()
	store.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).Return(database.APIKey{
		ID: keyID, UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
		AllowList: database.AllowList{{Type: "*", ID: "*"}},
	}, nil)
	store.EXPECT().GetUserByID(gomock.Any(), userID).Return(database.User{
		ID: userID, Username: "delegated-user", Status: database.UserStatusActive,
	}, nil)
	store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), userID).Return(database.GetAuthorizationUserRolesRow{
		ID: userID, Username: "delegated-user", Status: database.UserStatusActive,
		Roles: []string{orgMemberRole(orgID), "configured-model-user:" + orgID.String()},
	}, nil)
	store.EXPECT().CustomRoles(gomock.Any(), gomock.Any()).AnyTimes().Return(customModelRoles(orgID, []string{orgMemberRole(orgID)}), nil)

	store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{
		UserID: userID, ProviderName: "provider", Model: "model",
	}).Return([]database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, nil)

	srv, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
		Store:      store,
		Authorizer: rbac.NewStrictAuthorizer(prometheus.NewRegistry()),
		Logger:     slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	})
	require.NoError(t, err)

	resp, err := srv.IsAuthorized(t.Context(), &proto.IsAuthorizedRequest{
		KeyId: "delegated-model-key", ProviderName: stringPtr("provider"), Model: stringPtr("model"),
	})
	require.NoError(t, err)
	require.Equal(t, userID.String(), resp.GetOwnerId())
}

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
					}).Return([]database.GetAIModelAccessConfigsRow{{ID: modelID, OrganizationID: orgID}}, nil)
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
					require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
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
