package chatd

import (
	"context"
	"sync"
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
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestSyntheticAPIKeyReconcilesScopes(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	for _, scopes := range []struct {
		name  string
		value database.APIKeyScopes
	}{
		{"over_scoped", database.APIKeyScopes{database.ApiKeyScopeCoderAll}},
		{"empty", database.APIKeyScopes{}},
		{"extra_scope", database.APIKeyScopes{database.ApiKeyScopeApiKeyRead, database.ApiKeyScopeCoderAll}},
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
				want.Scopes = database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}
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

// Only unlocked reads pass through this barrier. Transactional rereads use
// the wrapped store's InTx, so all callers start with the same stale scopes.
type syntheticScopeReadBarrierStore struct {
	database.Store
	read    chan<- struct{}
	release <-chan struct{}
}

func (s syntheticScopeReadBarrierStore) GetChatGatewayAPIKey(ctx context.Context, arg database.GetChatGatewayAPIKeyParams) (database.APIKey, error) {
	key, err := s.Store.GetChatGatewayAPIKey(ctx, arg)
	select {
	case s.read <- struct{}{}:
	case <-ctx.Done():
		return database.APIKey{}, ctx.Err()
	}
	select {
	case <-s.release:
		return key, err
	case <-ctx.Done():
		return database.APIKey{}, ctx.Err()
	}
}

func TestSyntheticAPIKeyConcurrentScopeRepairs(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	for _, remaining := range []time.Duration{7 * 24 * time.Hour, time.Hour} {
		t.Run(remaining.String(), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			var wg sync.WaitGroup
			defer func() {
				cancel()
				wg.Wait()
			}()
			clock := quartz.NewMock(t)
			clock.Set(dbtime.Now()).MustWait(ctx)
			user := dbgen.User(t, db, database.User{})
			stale, _ := dbgen.APIKey(t, db, database.APIKey{
				UserID: user.ID, LoginType: user.LoginType,
				ExpiresAt: clock.Now().Add(remaining),
				Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
				TokenName: GatewayTokenName(user.ID),
			})
			const workers = 8
			read := make(chan struct{}, workers)
			release := make(chan struct{})
			server := &Server{
				db:    syntheticScopeReadBarrierStore{Store: authzDB, read: read, release: release},
				clock: clock,
			}
			ids := make([]string, workers)
			errs := make([]error, workers)
			for i := range workers {
				wg.Go(func() {
					ids[i], errs[i] = server.ensureSyntheticAPIKeyID(ctx, user.ID)
				})
			}
			for range workers {
				select {
				case <-read:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			close(release)
			wg.Wait()
			for i := range workers {
				require.NoError(t, errs[i])
				require.Equal(t, stale.ID, ids[i])
			}
			keys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
				UserID: user.ID, LoginType: user.LoginType, IncludeExpired: true,
			})
			require.NoError(t, err)
			want := stale
			want.Scopes = database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}
			if remaining <= syntheticAPIKeyRenewMargin {
				want.ExpiresAt = clock.Now().Add(syntheticAPIKeyLifetime).In(stale.ExpiresAt.Location())
			}
			require.Equal(t, []database.APIKey{want}, keys)
		})
	}
}
