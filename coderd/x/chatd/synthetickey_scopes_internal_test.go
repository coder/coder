package chatd

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/quartz"
)

func TestSyntheticAPIKeyReconcilesScopes(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	for _, scopes := range []struct {
		name  string
		value database.APIKeyScopes
		want  database.APIKeyScopes
	}{
		{"over_scoped", database.APIKeyScopes{database.ApiKeyScopeCoderAll}, database.APIKeyScopes{database.ApiKeyScopeCoderAll, database.ApiKeyScopeApiKeyRead}},
		{"empty", database.APIKeyScopes{}, database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}},
		{"extra_scope", database.APIKeyScopes{database.ApiKeyScopeApiKeyRead, database.ApiKeyScopeCoderAll}, database.APIKeyScopes{database.ApiKeyScopeApiKeyRead, database.ApiKeyScopeCoderAll}},
		{"extra_scope_reordered", database.APIKeyScopes{database.ApiKeyScopeCoderAll, database.ApiKeyScopeApiKeyRead}, database.APIKeyScopes{database.ApiKeyScopeCoderAll, database.ApiKeyScopeApiKeyRead}},
	} {
		for _, expiry := range []struct {
			name      string
			remaining time.Duration
			renew     bool
		}{
			{"healthy", 7 * 24 * time.Hour, false},
			{"renew_margin", syntheticAPIKeyRenewMargin, true},
			{"expired", -time.Hour, true},
		} {
			t.Run(scopes.name+"/"+expiry.name, func(t *testing.T) {
				t.Parallel()
				clock := quartz.NewMock(t)
				clock.Set(dbtime.Now()).MustWait(t.Context())
				user := dbgen.User(t, db, database.User{})
				// A bearer token with this unvalidated name must remain untouched,
				// even when its scopes and expiry also need reconciliation.
				collision, _ := dbgen.APIKey(t, db, database.APIKey{
					UserID: user.ID, LoginType: database.LoginTypeToken,
					TokenName: GatewayTokenName(user.ID), ExpiresAt: clock.Now().Add(-time.Hour),
					CreatedAt: clock.Now().Add(-30 * 24 * time.Hour),
					Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				})
				stale, _ := dbgen.APIKey(t, db, database.APIKey{
					UserID: user.ID, LoginType: user.LoginType,
					ExpiresAt: clock.Now().Add(expiry.remaining),
					AllowList: database.AllowList{{Type: rbac.ResourceWorkspace.Type, ID: uuid.NewString()}},
					TokenName: GatewayTokenName(user.ID),
				}, func(params *database.InsertAPIKeyParams) {
					// dbgen otherwise defaults empty scopes to coder:all.
					params.Scopes = scopes.value
				})
				server := &Server{db: authzDB, clock: clock}

				keyID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
				require.NoError(t, err)
				require.Equal(t, stale.ID, keyID)
				repaired, err := db.GetAPIKeyByID(t.Context(), keyID)
				require.NoError(t, err)
				want := stale
				want.Scopes = scopes.want
				if expiry.renew {
					want.ExpiresAt = clock.Now().Add(syntheticAPIKeyLifetime).In(stale.ExpiresAt.Location())
				}
				require.Equal(t, want, repaired, "only scopes and near-expiry expiration may change")
				secondID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
				require.NoError(t, err)
				require.Equal(t, keyID, secondID)
				again, err := db.GetAPIKeyByID(t.Context(), secondID)
				require.NoError(t, err)
				require.Equal(t, repaired, again)
				untouched, err := db.GetAPIKeyByID(t.Context(), collision.ID)
				require.NoError(t, err)
				require.Equal(t, collision, untouched)
			})
		}
	}
}
