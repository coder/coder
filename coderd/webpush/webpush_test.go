package webpush_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/webpush"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

const (
	validEndpointAuthKey   = "zqbxT6JKstKSY9JKibZLSQ=="
	validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="
)

type countingWebpushStore struct {
	database.Store
	getSubscriptionsCalls atomic.Int32
}

func (s *countingWebpushStore) GetWebpushSubscriptionsByUserID(ctx context.Context, userID uuid.UUID) ([]database.WebpushSubscription, error) {
	s.getSubscriptionsCalls.Add(1)
	return s.Store.GetWebpushSubscriptionsByUserID(ctx, userID)
}

func (s *countingWebpushStore) getCallCount() int32 {
	return s.getSubscriptionsCalls.Load()
}

func TestPush(t *testing.T) {
	t.Parallel()

	t.Run("SuccessfulDelivery", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		msg := randomWebpushMessage(t)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		})
		user := dbgen.User(t, store, database.User{})
		sub, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	t.Run("ExpiredSubscription", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusGone)
		})
		user := dbgen.User(t, store, database.User{})
		_, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 0, "No subscriptions should be returned")
	})

	t.Run("FailedDelivery", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))
		})

		user := dbgen.User(t, store, database.User{})
		sub, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid request")

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	t.Run("MultipleSubscriptions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		var okEndpointCalled bool
		var goneEndpointCalled bool
		manager, store, serverOKURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			okEndpointCalled = true
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		})

		serverGone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			goneEndpointCalled = true
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusGone)
		}))
		defer serverGone.Close()
		serverGoneURL := serverGone.URL

		// Setup subscriptions pointing to our test servers
		user := dbgen.User(t, store, database.User{})

		sub1, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverOKURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		_, err = store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverGoneURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)
		assert.True(t, okEndpointCalled, "The valid endpoint should be called")
		assert.True(t, goneEndpointCalled, "The expired endpoint should be called")

		// Assert that sub1 was not deleted.
		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		if assert.Len(t, subscriptions, 1, "One subscription should be returned") {
			assert.Equal(t, subscriptions[0].ID, sub1.ID, "The valid subscription should not be deleted")
		}
	})

	t.Run("NotificationPayload", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		var requestReceived bool
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			requestReceived = true
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		})

		user := dbgen.User(t, store, database.User{})

		_, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			CreatedAt:         dbtime.Now(),
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err, "Failed to insert push subscription")

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err, "The push notification should be dispatched successfully")
		require.True(t, requestReceived, "The push notification request should have been received by the server")
	})

	t.Run("NoSubscriptions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, _ := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		userID := uuid.New()
		notification := codersdk.WebpushMessage{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err := manager.Dispatch(ctx, userID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, userID)
		require.NoError(t, err)
		assert.Empty(t, subscriptions, "No subscriptions should be returned")
	})

	t.Run("CachesSubscriptionsWithinTTL", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		rawStore, _ := dbtestutil.NewDB(t)
		store := &countingWebpushStore{Store: rawStore}
		var delivered atomic.Int32
		manager, _, serverURL := setupPushTestWithOptions(ctx, t, store, func(w http.ResponseWriter, r *http.Request) {
			delivered.Add(1)
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		}, webpush.WithClock(clock), webpush.WithSubscriptionCacheTTL(time.Minute))

		user := dbgen.User(t, rawStore, database.User{})
		_, err := rawStore.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			CreatedAt:         dbtime.Now(),
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)

		require.Equal(t, int32(1), store.getCallCount(), "subscriptions should be read once within the TTL")
		require.Equal(t, int32(2), delivered.Load(), "both dispatches should send a notification")
	})

	t.Run("RefreshesSubscriptionsAfterTTLExpires", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		rawStore, _ := dbtestutil.NewDB(t)
		store := &countingWebpushStore{Store: rawStore}
		var delivered atomic.Int32
		manager, _, serverURL := setupPushTestWithOptions(ctx, t, store, func(w http.ResponseWriter, r *http.Request) {
			delivered.Add(1)
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		}, webpush.WithClock(clock), webpush.WithSubscriptionCacheTTL(time.Minute))

		user := dbgen.User(t, rawStore, database.User{})
		_, err := rawStore.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			CreatedAt:         dbtime.Now(),
			UserID:            user.ID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)
		clock.Advance(time.Minute)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)

		require.Equal(t, int32(2), store.getCallCount(), "dispatch should refresh subscriptions after the TTL expires")
		require.Equal(t, int32(2), delivered.Load(), "both dispatches should send a notification")
	})

	t.Run("PrunesStaleSubscriptionsFromCache", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		rawStore, _ := dbtestutil.NewDB(t)
		store := &countingWebpushStore{Store: rawStore}
		var okCalls atomic.Int32
		var goneCalls atomic.Int32
		manager, _, okServerURL := setupPushTestWithOptions(ctx, t, store, func(w http.ResponseWriter, r *http.Request) {
			okCalls.Add(1)
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusOK)
		}, webpush.WithClock(clock), webpush.WithSubscriptionCacheTTL(time.Minute))

		goneServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			goneCalls.Add(1)
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusGone)
		}))
		defer goneServer.Close()

		user := dbgen.User(t, rawStore, database.User{})
		okSubscription, err := rawStore.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			CreatedAt:         dbtime.Now(),
			UserID:            user.ID,
			Endpoint:          okServerURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err)
		_, err = rawStore.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			CreatedAt:         dbtime.Now(),
			UserID:            user.ID,
			Endpoint:          goneServer.URL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)
		err = manager.Dispatch(ctx, user.ID, msg)
		require.NoError(t, err)

		require.Equal(t, int32(1), store.getCallCount(), "stale subscription cleanup should not force a second DB read within the TTL")
		require.Equal(t, int32(2), okCalls.Load(), "the healthy endpoint should receive both dispatches")
		require.Equal(t, int32(1), goneCalls.Load(), "the stale endpoint should be pruned from the cache after the first dispatch")

		subscriptions, err := rawStore.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		require.Len(t, subscriptions, 1, "only the healthy subscription should remain")
		require.Equal(t, okSubscription.ID, subscriptions[0].ID)
	})
}

func randomWebpushMessage(t testing.TB) codersdk.WebpushMessage {
	t.Helper()
	return codersdk.WebpushMessage{
		Title: testutil.GetRandomName(t),
		Body:  testutil.GetRandomName(t),

		Actions: []codersdk.WebpushMessageAction{
			{Label: "A", URL: "https://example.com/a"},
			{Label: "B", URL: "https://example.com/b"},
		},
		Icon: "https://example.com/icon.png",
	}
}

func assertWebpushPayload(t testing.TB, r *http.Request) {
	t.Helper()
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
	assert.Equal(t, r.Header.Get("content-encoding"), "aes128gcm")
	assert.Contains(t, r.Header.Get("Authorization"), "vapid")

	// Attempting to decode the request body as JSON should fail as it is
	// encrypted.
	assert.Error(t, json.NewDecoder(r.Body).Decode(io.Discard))
}

// setupPushTest creates a common test setup for webpush notification tests.
func setupPushTest(ctx context.Context, t *testing.T, handlerFunc func(w http.ResponseWriter, r *http.Request)) (webpush.Dispatcher, database.Store, string) {
	t.Helper()
	db, _ := dbtestutil.NewDB(t)
	return setupPushTestWithOptions(ctx, t, db, handlerFunc)
}

func setupPushTestWithOptions(ctx context.Context, t *testing.T, db database.Store, handlerFunc func(w http.ResponseWriter, r *http.Request), opts ...webpush.Option) (webpush.Dispatcher, database.Store, string) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	server := httptest.NewServer(http.HandlerFunc(handlerFunc))
	t.Cleanup(server.Close)

	manager, err := webpush.New(ctx, &logger, db, "http://example.com", opts...)
	require.NoError(t, err, "Failed to create webpush manager")

	return manager, db, server.URL
}
