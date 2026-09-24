package aibridgedserver_test

import (
	"database/sql"
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
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/rbac"
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
				store.EXPECT().GetAIModelAccessConfigs(gomock.Any(), database.GetAIModelAccessConfigsParams{
					UserID: userID, ProviderName: *tc.provider, Model: *tc.model,
				}).Return(tc.configs, tc.queryErr).AnyTimes()
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
func TestIsAuthorizedDelegatedModelAuthorizationRespectsScope(t *testing.T) {
	t.Parallel()

	store := dbmock.NewMockStore(gomock.NewController(t))
	userID := uuid.New()
	keyID := "delegated-scoped-model-key"
	orgID := uuid.New()
	modelID := uuid.New()
	store.EXPECT().GetAPIKeyByID(gomock.Any(), keyID).Return(database.APIKey{
		ID:        keyID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour),
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
		AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}},
	}, nil)
	store.EXPECT().GetUserByID(gomock.Any(), userID).Return(database.User{
		ID: userID, Username: "delegated-user", Status: database.UserStatusActive,
	}, nil)
	store.EXPECT().GetAuthorizationUserRoles(gomock.Any(), userID).Return(database.GetAuthorizationUserRolesRow{
		ID: userID, Username: "delegated-user", Status: database.UserStatusActive,
		Roles: []string{orgMemberRole(orgID)},
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
		KeyId: keyID, ProviderName: stringPtr("provider"), Model: stringPtr("model"),
	})
	require.Error(t, err)
	require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
	require.Nil(t, resp)
}
