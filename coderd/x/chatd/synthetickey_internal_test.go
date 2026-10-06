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

func TestSyntheticAPIKeyLifecycle(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now()).MustWait(t.Context())
	server := &Server{db: db, clock: clock}

	firstID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)

	first, err := db.GetAPIKeyByID(t.Context(), firstID)
	require.NoError(t, err)
	require.Equal(t, user.LoginType, first.LoginType)
	require.Equal(t, GatewayTokenName(user.ID), first.TokenName)
	require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}, first.Scopes)
	require.True(t, first.ExpiresAt.Equal(clock.Now().Add(syntheticAPIKeyLifetime)))

	clock.Advance(syntheticAPIKeyLifetime - syntheticAPIKeyRenewMargin - time.Second).MustWait(t.Context())
	secondID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
	require.NoError(t, err)
	require.Equal(t, firstID, secondID)
	unchanged, err := db.GetAPIKeyByID(t.Context(), secondID)
	require.NoError(t, err)
	require.Equal(t, first, unchanged)

	// Near-expiry and fully expired keys are extended in place: in-flight
	// generations may have delegated the ID already.
	for _, advance := range []time.Duration{2 * time.Second, syntheticAPIKeyLifetime + time.Second} {
		clock.Advance(advance).MustWait(t.Context())

		renewedID, err := server.ensureSyntheticAPIKeyID(t.Context(), user.ID)
		require.NoError(t, err)
		require.Equal(t, firstID, renewedID)
		renewed, err := db.GetAPIKeyByID(t.Context(), renewedID)
		require.NoError(t, err)
		want := first
		// Match the database time zone for the whole-row comparison.
		want.ExpiresAt = clock.Now().Add(syntheticAPIKeyLifetime).In(first.ExpiresAt.Location())
		require.Equal(t, want, renewed)
	}

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
	require.Equal(t, collision, unchanged)
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

// Hold completed unlocked reads until every caller has observed the missing key.
// Transactional rereads bypass this barrier.
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

func TestSyntheticAPIKeyConcurrentMint(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	// Exercise minting through the Chatd key minter's authorization boundary.
	authzDB := dbauthz.New(db, rbac.NewStrictAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), nil)
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	var wg sync.WaitGroup
	// Join workers even if the barrier times out, before fixture cleanup.
	defer func() {
		cancel()
		wg.Wait()
	}()
	clock := quartz.NewMock(t)
	clock.Set(dbtime.Now()).MustWait(ctx)
	user := dbgen.User(t, db, database.User{})
	const workers = 8
	read := make(chan struct{}, workers)
	release := make(chan struct{})
	server := &Server{
		db:    syntheticKeyReadBarrierStore{Store: authzDB, read: read, release: release},
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
		require.Equal(t, ids[0], ids[i])
	}
	keys, err := db.GetAPIKeysByUserID(ctx, database.GetAPIKeysByUserIDParams{
		UserID: user.ID, LoginType: user.LoginType, IncludeExpired: true,
	})
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, keys[0].ID, ids[0])
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
