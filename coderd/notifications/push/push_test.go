package push_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications/push"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	validEndpointAuthKey   = "zqbxT6JKstKSY9JKibZLSQ=="
	validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="
)

func TestPush(t *testing.T) {
	t.Parallel()

	t.Run("SuccessfulDelivery", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
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

		notification := codersdk.WebpushMessage{
			Title: "Test Title",
			Body:  "Test Body",
			Actions: []codersdk.WebpushMessageAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
			Icon: "workspace",
		}

		err = manager.Dispatch(ctx, user.ID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	t.Run("ExpiredSubscription", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
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

		notification := codersdk.WebpushMessage{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err = manager.Dispatch(ctx, user.ID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetWebpushSubscriptionsByUserID(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 0, "No subscriptions should be returned")
	})

	t.Run("FailedDelivery", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
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

		notification := codersdk.WebpushMessage{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err = manager.Dispatch(ctx, user.ID, notification)
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
		manager, store, serverOKURL := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
			okEndpointCalled = true
			w.WriteHeader(http.StatusOK)
		})

		serverGone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			goneEndpointCalled = true
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

		notification := codersdk.WebpushMessage{
			Title: "Test Title",
			Body:  "Test Body",
			Actions: []codersdk.WebpushMessageAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
		}

		err = manager.Dispatch(ctx, user.ID, notification)
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
		manager, store, serverURL := setupPushTest(ctx, t, func(w http.ResponseWriter, _ *http.Request) {
			requestReceived = true
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

		notification := codersdk.WebpushMessage{
			Title: "Test Notification",
			Body:  "This is a test notification body",
			Actions: []codersdk.WebpushMessageAction{
				{Label: "View Workspace", URL: "https://coder.com/workspace/123"},
				{Label: "Cancel", URL: "https://coder.com/cancel"},
			},
			Icon: "workspace-icon",
		}

		err = manager.Dispatch(ctx, user.ID, notification)
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
}

// setupPushTest creates a common test setup for push notification tests
func setupPushTest(ctx context.Context, t *testing.T, handlerFunc func(w http.ResponseWriter, r *http.Request)) (push.NotificationDispatcher, database.Store, string) {
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	db, _ := dbtestutil.NewDB(t)

	server := httptest.NewServer(http.HandlerFunc(handlerFunc))
	t.Cleanup(server.Close)

	manager, err := push.New(ctx, &logger, db)
	require.NoError(t, err, "Failed to create push manager")

	return manager, db, server.URL
}
