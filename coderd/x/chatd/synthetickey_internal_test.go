package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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
	require.Equal(t, database.APIKeyScopes{database.ApiKeyScopeApiKeyRead}, first.Scopes)
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
