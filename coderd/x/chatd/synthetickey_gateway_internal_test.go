package chatd

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"storj.io/drpc/drpcerr"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/coderd/aibridgedserver"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/quartz"
)

func TestSyntheticAPIKeyGatewayContinuity(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, nil)
	auth := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
	authzDB := dbauthz.New(db, auth, logger, nil)
	gateway, err := aibridgedserver.NewServer(t.Context(), aibridgedserver.Options{
		Store: authzDB, Authorizer: auth, Logger: logger,
	})
	require.NoError(t, err)
	org := dbgen.Organization(t, db, database.Organization{})
	provider := dbgen.AIProvider(t, db, database.AIProvider{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		OrganizationID: org.ID,
		AIProviderID:   uuid.NullUUID{UUID: provider.ID, Valid: true},
	})

	for _, grant := range []string{"configured", "unrestricted"} {
		for _, legacyScope := range []database.APIKeyScope{database.ApiKeyScopeApiKeyRead, database.ApiKeyScopeCoderAll} {
			t.Run(grant+"/"+string(legacyScope), func(t *testing.T) {
				t.Parallel()
				clock := quartz.NewMock(t)
				clock.Set(dbtime.Now()).MustWait(t.Context())
				owner := dbgen.User(t, db, database.User{})
				modelName := model.Model
				if grant == "configured" {
					dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: owner.ID})
				} else {
					// No organization membership or configured model can
					// provide the grant in the unrestricted case.
					modelName = uuid.NewString()
					_, err := db.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{
						ID: owner.ID, GrantedRoles: []string{rbac.RoleAIGatewayUnrestricted().Name},
					})
					require.NoError(t, err)
				}
				legacy, _ := dbgen.APIKey(t, db, database.APIKey{
					UserID: owner.ID, LoginType: owner.LoginType,
					TokenName: GatewayTokenName(owner.ID),
					ExpiresAt: clock.Now().Add(7 * 24 * time.Hour),
					Scopes:    database.APIKeyScopes{legacyScope},
				})

				// Keep the transport created before the upgrade, as an
				// in-flight generation would. The base invokes the real
				// Gateway authorization with its validated delegated ID.
				transport := &aiGatewayRoundTripper{
					apiKeyID: legacy.ID,
					base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						keyID, ok := aibridge.DelegatedAPIKeyIDFromContext(req.Context())
						require.True(t, ok)
						require.Equal(t, legacy.ID, keyID)
						resp, err := gateway.IsAuthorized(req.Context(), &proto.IsAuthorizedRequest{
							KeyId: keyID, ProviderName: &provider.Name, Model: &modelName,
						})
						if err != nil {
							return nil, err
						}
						require.Equal(t, legacy.ID, resp.GetApiKeyId())
						require.Equal(t, owner.ID.String(), resp.GetOwnerId())
						return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
					}),
				}
				request := func() error {
					req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, aibridgeLocalBaseURL+"/v1/chat/completions", http.NoBody)
					require.NoError(t, err)
					resp, err := transport.RoundTrip(req)
					if err == nil {
						require.Equal(t, http.StatusOK, resp.StatusCode)
						require.NoError(t, resp.Body.Close())
					}
					return err
				}
				if legacyScope == database.ApiKeyScopeApiKeyRead {
					err := request()
					require.Error(t, err)
					require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
				} else {
					require.NoError(t, request())
				}

				server := &Server{db: authzDB, clock: clock}
				keyID, err := server.ensureSyntheticAPIKeyID(t.Context(), owner.ID)
				require.NoError(t, err)
				require.Equal(t, legacy.ID, keyID)
				repaired, err := db.GetAPIKeyByID(t.Context(), keyID)
				require.NoError(t, err)
				require.Contains(t, repaired.Scopes, database.ApiKeyScopeAIGatewayUnrestrictedUse)
				require.NoError(t, request(), "the original transport's next request must authorize after a scope upgrade")
				if grant == "configured" {
					modelName = uuid.NewString()
					err = request()
					require.Error(t, err, "the unrestricted-use scope must not grant the owner unrestricted access")
					require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
					modelName = model.Model
				}

				require.NoError(t, db.UpdateAPIKeyByID(t.Context(), database.UpdateAPIKeyByIDParams{
					ID: legacy.ID, LastUsed: legacy.LastUsed, IPAddress: legacy.IPAddress,
					ExpiresAt: clock.Now().Add(time.Hour),
				}))
				renewedID, err := server.ensureSyntheticAPIKeyID(t.Context(), owner.ID)
				require.NoError(t, err)
				require.Equal(t, legacy.ID, renewedID)
				require.NoError(t, request(), "the original transport must also survive renewal")

				// The minimal scopes permit, but do not grant, model use.
				if grant == "configured" {
					require.NoError(t, db.DeleteOrganizationMember(t.Context(), database.DeleteOrganizationMemberParams{
						OrganizationID: org.ID, UserID: owner.ID,
					}))
				} else {
					_, err := db.UpdateUserRoles(t.Context(), database.UpdateUserRolesParams{ID: owner.ID, GrantedRoles: []string{}})
					require.NoError(t, err)
				}
				err = request()
				require.Error(t, err)
				require.Equal(t, proto.AuthorizationErrorPolicy, drpcerr.Code(err))
				afterRevocation, err := db.GetAPIKeyByID(t.Context(), keyID)
				require.NoError(t, err)
				require.Equal(t, repaired.Scopes, afterRevocation.Scopes, "revocation is enforced by current RBAC permissions, not scope removal")
			})
		}
	}
}
