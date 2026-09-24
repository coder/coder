package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
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

func getGatewayKey(ctx context.Context, db database.Store, userID uuid.UUID) (database.APIKey, error) {
	return db.GetChatGatewayAPIKey(ctx, database.GetChatGatewayAPIKeyParams{
		UserID:    userID,
		TokenName: GatewayTokenName(userID),
	})
}

func TestSyntheticAPIKeyUpgradesLegacyScopes(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	for _, scopes := range []struct {
		name  string
		value database.APIKeyScopes
	}{
		{"read_only", database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}},
		{"all", database.APIKeyScopes{database.ApiKeyScopeCoderAll}},
	} {
		for _, expiry := range []struct {
			name      string
			remaining time.Duration
			renew     bool
		}{
			{"healthy", 7 * 24 * time.Hour, false},
			{"renew_margin", syntheticAPIKeyRenewMargin, true},
			{"near_expiry", time.Hour, true},
			{"expired", -time.Hour, true},
		} {
			t.Run(scopes.name+"/"+expiry.name, func(t *testing.T) {
				t.Parallel()
				clock := quartz.NewMock(t)
				clock.Set(dbtime.Now()).MustWait(t.Context())
				user := dbgen.User(t, db, database.User{})
				legacy, _ := dbgen.APIKey(t, db, database.APIKey{
					UserID:    user.ID,
					LoginType: user.LoginType,
					ExpiresAt: clock.Now().Add(expiry.remaining),
					Scopes:    scopes.value,
					AllowList: database.AllowList{{Type: rbac.ResourceChatModelConfig.Type, ID: uuid.NewString()}},
					TokenName: GatewayTokenName(user.ID),
				})
				server := &Server{db: authzDB, clock: clock}

				keyID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
				require.NoError(t, err)
				require.Equal(t, legacy.ID, keyID)
				upgraded, err := db.GetAPIKeyByID(t.Context(), legacy.ID)
				require.NoError(t, err)
				want := legacy
				want.Scopes = database.APIKeyScopes{
					database.ApiKeyScopeApiKeyRead,
					database.ApiKeyScopeChatModelConfigUse,
					database.ApiKeyScopeAIGatewayUnrestrictedUse,
				}
				if expiry.renew {
					want.ExpiresAt = clock.Now().Add(syntheticAPIKeyLifetime).In(legacy.ExpiresAt.Location())
				}
				require.Equal(t, want, upgraded, "only scopes and near-expiry expiration may change")
				secondID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
				require.NoError(t, err)
				require.Equal(t, legacy.ID, secondID)
				again, err := db.GetAPIKeyByID(t.Context(), secondID)
				require.NoError(t, err)
				require.Equal(t, upgraded, again)
			})
		}
	}
}

func TestSyntheticAPIKeyLifecycle(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	server := &Server{db: db, clock: quartz.NewReal()}

	firstID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	secondID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, firstID, secondID)

	first, err := db.GetAPIKeyByID(t.Context(), firstID)
	require.NoError(t, err)
	require.Equal(t, user.LoginType, first.LoginType)
	require.Equal(t, GatewayTokenName(user.ID), first.TokenName)
	require.Equal(t, syntheticAPIKeyScopes, first.Scopes)
	require.WithinDuration(t, server.clock.Now().Add(syntheticAPIKeyLifetime), first.ExpiresAt, time.Second)

	// Within the renew margin the key is extended in place: the ID stays
	// stable because in-flight generations may have delegated it already.
	err = db.UpdateAPIKeyByID(t.Context(), database.UpdateAPIKeyByIDParams{
		ID:        first.ID,
		LastUsed:  first.LastUsed,
		ExpiresAt: server.clock.Now().Add(time.Hour),
		IPAddress: first.IPAddress,
	})
	require.NoError(t, err)

	renewedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, firstID, renewedID)
	renewed, err := db.GetAPIKeyByID(t.Context(), renewedID)
	require.NoError(t, err)
	require.WithinDuration(t, server.clock.Now().Add(syntheticAPIKeyLifetime), renewed.ExpiresAt, time.Second)

	// A fully expired key is extended the same way.
	err = db.UpdateAPIKeyByID(t.Context(), database.UpdateAPIKeyByIDParams{
		ID:        first.ID,
		LastUsed:  first.LastUsed,
		ExpiresAt: server.clock.Now().Add(-time.Hour),
		IPAddress: first.IPAddress,
	})
	require.NoError(t, err)

	revivedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, firstID, revivedID)
	revived, err := db.GetAPIKeyByID(t.Context(), revivedID)
	require.NoError(t, err)
	require.WithinDuration(t, server.clock.Now().Add(syntheticAPIKeyLifetime), revived.ExpiresAt, time.Second)

	// External deletion (password reset, dbpurge) causes a remint.
	require.NoError(t, db.DeleteAPIKeyByID(t.Context(), firstID))
	_, err = getGatewayKey(t.Context(), db, user.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	recreatedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.NotEqual(t, firstID, recreatedID)
}

func TestSyntheticAPIKeyIgnoresUserTokenCollision(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	server := &Server{db: db, clock: quartz.NewReal()}

	// Token names are unvalidated user input, so a user can create a token
	// named exactly like the synthetic gateway key. It must never be picked
	// up or extended; its near-margin expiry would otherwise trigger the
	// extension path.
	collisionExpiry := server.clock.Now().Add(time.Hour).UTC()
	collision, _ := dbgen.APIKey(t, db, database.APIKey{
		UserID:    user.ID,
		LoginType: database.LoginTypeToken,
		TokenName: GatewayTokenName(user.ID),
		ExpiresAt: collisionExpiry,
	})

	syntheticID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.NotEqual(t, collision.ID, syntheticID)

	synthetic, err := db.GetAPIKeyByID(t.Context(), syntheticID)
	require.NoError(t, err)
	require.Equal(t, user.LoginType, synthetic.LoginType)

	unchanged, err := db.GetAPIKeyByID(t.Context(), collision.ID)
	require.NoError(t, err)
	require.WithinDuration(t, collisionExpiry, unchanged.ExpiresAt, time.Millisecond)
	require.Equal(t, collision, unchanged)

	// A scope upgrade must also leave the colliding bearer token untouched.
	legacy, err := db.UpdateChatGatewayAPIKeyScopesByID(t.Context(), database.UpdateChatGatewayAPIKeyScopesByIDParams{
		ID: syntheticID, UserID: user.ID, Scopes: database.APIKeyScopes{database.ApiKeyScopeApiKeyRead},
	})
	require.NoError(t, err)
	upgradedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, legacy.ID, upgradedID)
	unchanged, err = db.GetAPIKeyByID(t.Context(), collision.ID)
	require.NoError(t, err)
	require.Equal(t, collision, unchanged)
}

// Only the unlocked reads pass through this wrapper. Transactional rereads
// use the wrapped store's InTx, so all callers start with the legacy scopes.
type syntheticKeyReadBarrierStore struct {
	database.Store
	read    chan<- struct{}
	release <-chan struct{}
}

func (s syntheticKeyReadBarrierStore) GetChatGatewayAPIKey(ctx context.Context, arg database.GetChatGatewayAPIKeyParams) (database.APIKey, error) {
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

func TestSyntheticAPIKeyConcurrentUpgrade(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now()).MustWait(ctx)
	user := dbgen.User(t, db, database.User{})
	legacy, _ := dbgen.APIKey(t, db, database.APIKey{
		UserID: user.ID, LoginType: user.LoginType,
		ExpiresAt: clock.Now().Add(time.Hour),
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeApiKeyRead},
		TokenName: GatewayTokenName(user.ID),
	})
	const workers = 8
	read := make(chan struct{}, workers)
	release := make(chan struct{})
	server := &Server{
		db:    syntheticKeyReadBarrierStore{Store: authzDB, read: read, release: release},
		clock: clock,
	}
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
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
		require.Equal(t, legacy.ID, ids[i])
	}
	keys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		UserID: user.ID, LoginType: user.LoginType, IncludeExpired: true,
	})
	require.NoError(t, err)
	require.Len(t, keys, 1)
	want := legacy
	want.Scopes = database.APIKeyScopes{
		database.ApiKeyScopeApiKeyRead,
		database.ApiKeyScopeChatModelConfigUse,
		database.ApiKeyScopeAIGatewayUnrestrictedUse,
	}
	want.ExpiresAt = clock.Now().Add(syntheticAPIKeyLifetime).In(legacy.ExpiresAt.Location())
	require.Equal(t, want, keys[0])
}

func TestSyntheticAPIKeySurvivesSuspension(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	server := &Server{db: db, clock: quartz.NewReal()}

	keyID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)

	_, err = db.UpdateUserStatus(t.Context(), database.UpdateUserStatusParams{
		ID:        user.ID,
		Status:    database.UserStatusSuspended,
		UpdatedAt: dbtime.Now(),
	})
	require.NoError(t, err)

	// Suspension does not delete the key. Delegated gateway authorization
	// rejects suspended owners at request time instead; see the
	// aibridgedserver IsAuthorized tests for that rejection.
	_, err = db.GetAPIKeyByID(t.Context(), keyID)
	require.NoError(t, err)
	sameID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, keyID, sameID)
}

func TestSyntheticAPIKeyConcurrentMint(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	server := &Server{db: db, clock: quartz.NewReal()}

	const workers = 8
	ids := make([]string, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
		}()
	}
	wg.Wait()

	for i := range workers {
		require.NoError(t, errs[i])
		require.Equal(t, ids[0], ids[i])
	}
	keys, err := db.GetAPIKeysByUserID(t.Context(), database.GetAPIKeysByUserIDParams{
		LoginType:      user.LoginType,
		UserID:         user.ID,
		IncludeExpired: true,
	})
	require.NoError(t, err)
	require.Len(t, keys, 1)
}

func TestSyntheticAPIKeyDeletionDoesNotMutateChatState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		deleteKey func(context.Context, database.Store, uuid.UUID, string) error
	}{
		{
			name: "individual",
			deleteKey: func(ctx context.Context, db database.Store, _ uuid.UUID, keyID string) error {
				return db.DeleteAPIKeyByID(ctx, keyID)
			},
		},
		{
			name: "all user keys",
			deleteKey: func(ctx context.Context, db database.Store, userID uuid.UUID, _ string) error {
				return db.DeleteAPIKeysByUserID(ctx, userID)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
			server := &Server{db: db, clock: quartz.NewReal()}
			syntheticID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
			require.NoError(t, err)

			chat := dbgen.Chat(t, db, database.Chat{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				LastModelConfigID: model.ID,
			})
			dbgen.ChatMessage(t, db, database.ChatMessage{
				ChatID:        chat.ID,
				CreatedBy:     uuid.NullUUID{UUID: user.ID, Valid: true},
				ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
				Role:          database.ChatMessageRoleUser,
			})
			_, err = db.InsertChatQueuedMessage(t.Context(), database.InsertChatQueuedMessageParams{
				ChatID:        chat.ID,
				Content:       json.RawMessage(`[]`),
				ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
			})
			require.NoError(t, err)

			before, err := db.GetChatByID(t.Context(), chat.ID)
			require.NoError(t, err)
			require.NoError(t, test.deleteKey(t.Context(), db, user.ID, syntheticID))

			_, err = db.GetAPIKeyByID(t.Context(), syntheticID)
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = getGatewayKey(t.Context(), db, user.ID)
			require.ErrorIs(t, err, sql.ErrNoRows)

			after, err := db.GetChatByID(t.Context(), chat.ID)
			require.NoError(t, err)
			require.Equal(t, before.HistoryVersion, after.HistoryVersion)
			require.Equal(t, before.QueueVersion, after.QueueVersion)
			require.Equal(t, before.GenerationAttempt, after.GenerationAttempt)

			remintedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
			require.NoError(t, err)
			require.NotEqual(t, syntheticID, remintedID)
		})
	}
}
