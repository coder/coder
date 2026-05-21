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
		// 5xx responses are transient failures. The subscription should
		// remain after a failed delivery so it can be retried later.
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
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
		assert.Contains(t, err.Error(), "Internal server error")

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	// StaleSubscriptionStatuses verifies that documented permanent-failure
	// status codes from the push service cause the subscription to be
	// deleted. iOS Safari returns 404 and 403 BadJwtToken for invalidated
	// subscriptions, FCM returns 404 for endpoints that are no longer
	// valid, and a 400 means the subscription cannot be used.
	t.Run("StaleSubscriptionStatuses", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name           string
			statusCode     int
			body           string
			expectError    bool
			expectErrorMsg string
		}{
			{
				name:           "NotFound",
				statusCode:     http.StatusNotFound,
				body:           "Not Found",
				expectError:    true,
				expectErrorMsg: "Not Found",
			},
			{
				name:           "Forbidden",
				statusCode:     http.StatusForbidden,
				body:           "BadJwtToken",
				expectError:    true,
				expectErrorMsg: "BadJwtToken",
			},
			{
				name:           "BadRequest",
				statusCode:     http.StatusBadRequest,
				body:           "Invalid request",
				expectError:    true,
				expectErrorMsg: "Invalid request",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitShort)
				manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
					assertWebpushPayload(t, r)
					w.WriteHeader(tc.statusCode)
					w.Write([]byte(tc.body))
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
				if tc.expectError {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.expectErrorMsg)
				} else {
					require.NoError(t, err)
				}

				subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
				require.NoError(t, err)
				assert.Len(t, subscriptions, 0, "Stale subscription should be deleted on %d", tc.statusCode)
			})
		}
	})

	// StaleAndFailedSubscriptions verifies that a stale subscription
	// returning 404 is cleaned up even when a sibling subscription's
	// delivery fails with a transient error in the same Dispatch call.
	// Regression test for the case where a delivery error short-circuits
	// stale subscription cleanup, leaving permanently invalid rows in
	// the database.
	t.Run("StaleAndFailedSubscriptions", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		manager, store, server500URL := setupPushTest(ctx, t, func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("transient error"))
		})

		serverStale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertWebpushPayload(t, r)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer serverStale.Close()
		serverStaleURL := serverStale.URL

		user := dbgen.User(t, store, database.User{})

		subFailed, err := store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          server500URL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		_, err = store.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
			UserID:            user.ID,
			Endpoint:          serverStaleURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		msg := randomWebpushMessage(t)
		err = manager.Dispatch(ctx, user.ID, msg)
		// Should still surface a delivery error from one of the
		// failing siblings. errgroup returns whichever goroutine
		// finishes with an error first, so the error may originate
		// from either the 500 or the 404 sibling. The contract we
		// care about is that the stale (404) subscription is
		// cleaned up regardless of which error wins the race.
		require.Error(t, err)

		// The stale subscription should have been cleaned up regardless.
		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		if assert.Len(t, subscriptions, 1, "Only the transiently failing subscription should remain") {
			assert.Equal(t, subFailed.ID, subscriptions[0].ID, "The transiently failing subscription should not be deleted")
		}
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
// The test HTTP client bypasses SSRF protection so that httptest.Server
// (bound to 127.0.0.1) can be reached.
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

	// Use an unrestricted HTTP client for tests. The default SSRF-safe
	// client rejects loopback addresses, which blocks httptest.Server.
	opts = append(opts, webpush.WithHTTPClient(http.DefaultClient))
	manager, err := webpush.New(ctx, &logger, db, "http://example.com", opts...)
	require.NoError(t, err, "Failed to create webpush manager")

	return manager, db, server.URL
}

func TestNoopWebpusher(t *testing.T) {
	t.Parallel()

	noop := &webpush.NoopWebpusher{
		Msg: "push disabled",
	}

	dispatchErr := noop.Dispatch(context.Background(), uuid.New(), codersdk.WebpushMessage{})
	require.Error(t, dispatchErr)
	require.Contains(t, dispatchErr.Error(), "push disabled")

	testErr := noop.Test(context.Background(), codersdk.WebpushSubscription{})
	require.Error(t, testErr)
	require.Contains(t, testErr.Error(), "push disabled")

	require.Empty(t, noop.PublicKey())
}

// TestSSRFPrevention verifies that the default SSRF-safe HTTP client blocks
// webpush delivery to loopback (and other non-public) addresses. This
// reproduces the attack vector from the original SSRF PoC: an authenticated
// user supplies a localhost endpoint in their webpush subscription, and the
// server must refuse to connect.
func TestSSRFPrevention(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	// Start a server that records whether it received a request.
	var received atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Store(true)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	// Create a dispatcher via New() WITHOUT WithHTTPClient so it
	// uses the default SSRF-safe client that blocks loopback.
	db, _ := dbtestutil.NewDB(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	manager, err := webpush.New(ctx, &logger, db, "http://example.com")
	require.NoError(t, err)

	// Test() calls webpushSend directly with the supplied endpoint.
	err = manager.Test(ctx, codersdk.WebpushSubscription{
		Endpoint:  server.URL,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.Error(t, err, "SSRF-safe client should reject Test() to loopback address")
	assert.False(t, received.Load(), "Test() request should not reach the localhost server")

	// Dispatch() goes through the subscription cache → webpushSend path.
	user := dbgen.User(t, db, database.User{})
	_, err = db.InsertWebpushSubscription(ctx, database.InsertWebpushSubscriptionParams{
		CreatedAt:         dbtime.Now(),
		UserID:            user.ID,
		Endpoint:          server.URL,
		EndpointAuthKey:   validEndpointAuthKey,
		EndpointP256dhKey: validEndpointP256dhKey,
	})
	require.NoError(t, err)

	err = manager.Dispatch(ctx, user.ID, codersdk.WebpushMessage{
		Title: "SSRF test",
		Body:  "This should not arrive.",
	})
	require.Error(t, err, "SSRF-safe client should reject Dispatch() to loopback address")
	assert.False(t, received.Load(), "Dispatch() request should not reach the localhost server")
}
