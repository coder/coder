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
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications/push"
	"github.com/coder/coder/v2/codersdk"
)

const validEndpointAuthKey = "zqbxT6JKstKSY9JKibZLSQ=="
const validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="

func TestPush(t *testing.T) {
	t.Parallel()

	t.Run("SuccessfulDelivery", func(t *testing.T) {
		t.Parallel()
		manager, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		userID := uuid.New()
		sub, err := store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                uuid.New(),
			UserID:            userID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		notification := codersdk.PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
			Actions: []codersdk.PushNotificationAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
			Icon: "workspace",
		}

		err = manager.Dispatch(context.Background(), userID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetNotificationPushSubscriptionsByUserID(context.Background(), userID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	t.Run("ExpiredSubscription", func(t *testing.T) {
		t.Parallel()
		manager, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusGone)
		})
		userID := uuid.New()
		subID := uuid.New()
		_, err := store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                subID,
			UserID:            userID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		notification := codersdk.PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err = manager.Dispatch(context.Background(), userID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetNotificationPushSubscriptionsByUserID(context.Background(), userID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 0, "No subscriptions should be returned")
	})

	t.Run("FailedDelivery", func(t *testing.T) {
		t.Parallel()
		manager, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))
		})
		userID := uuid.New()
		sub, err := store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                uuid.New(),
			UserID:            userID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		notification := codersdk.PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err = manager.Dispatch(context.Background(), userID, notification)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid request")

		subscriptions, err := store.GetNotificationPushSubscriptionsByUserID(context.Background(), userID)
		require.NoError(t, err)
		assert.Len(t, subscriptions, 1, "One subscription should be returned")
		assert.Equal(t, subscriptions[0].ID, sub.ID, "The subscription should not be deleted")
	})

	t.Run("MultipleSubscriptions", func(t *testing.T) {
		t.Parallel()

		var okEndpointCalled bool
		var goneEndpointCalled bool
		manager, store, serverOKURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
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
		userID := uuid.New()
		sub1ID := uuid.New()
		sub2ID := uuid.New()

		_, err := store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                sub1ID,
			UserID:            userID,
			Endpoint:          serverOKURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		_, err = store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                sub2ID,
			UserID:            userID,
			Endpoint:          serverGoneURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
			CreatedAt:         dbtime.Now(),
		})
		require.NoError(t, err)

		notification := codersdk.PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
			Actions: []codersdk.PushNotificationAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
		}

		err = manager.Dispatch(context.Background(), userID, notification)
		require.NoError(t, err)
		assert.True(t, okEndpointCalled, "The valid endpoint should be called")
		assert.True(t, goneEndpointCalled, "The expired endpoint should be called")

		// assert.Len(t, store.deletedIDs, 1, "One subscription should be deleted")
		// assert.Contains(t, store.deletedIDs, sub2ID, "The expired subscription should be deleted")
		// assert.NotContains(t, store.deletedIDs, sub1ID, "The valid subscription should not be deleted")
	})

	t.Run("NotificationPayload", func(t *testing.T) {
		t.Parallel()
		var requestReceived bool
		manager, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			requestReceived = true
			w.WriteHeader(http.StatusOK)
		})

		userID := uuid.New()

		_, err := store.InsertNotificationPushSubscription(context.Background(), database.InsertNotificationPushSubscriptionParams{
			ID:                uuid.New(),
			CreatedAt:         dbtime.Now(),
			UserID:            userID,
			Endpoint:          serverURL,
			EndpointAuthKey:   validEndpointAuthKey,
			EndpointP256dhKey: validEndpointP256dhKey,
		})
		require.NoError(t, err)

		notification := codersdk.PushNotification{
			Title: "Test Notification",
			Body:  "This is a test notification body",
			Actions: []codersdk.PushNotificationAction{
				{Label: "View Workspace", URL: "https://coder.com/workspace/123"},
				{Label: "Cancel", URL: "https://coder.com/cancel"},
			},
			Icon: "workspace-icon",
		}

		err = manager.Dispatch(context.Background(), userID, notification)
		require.NoError(t, err)
		assert.True(t, requestReceived, "The push notification request should have been received by the server")
	})

	t.Run("NoSubscriptions", func(t *testing.T) {
		t.Parallel()
		manager, store, _ := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		userID := uuid.New()
		notification := codersdk.PushNotification{
			Title: "Test Title",
			Body:  "Test Body",
		}

		err := manager.Dispatch(context.Background(), userID, notification)
		require.NoError(t, err)

		subscriptions, err := store.GetNotificationPushSubscriptionsByUserID(context.Background(), userID)
		require.NoError(t, err)
		assert.Empty(t, subscriptions, "No subscriptions should be returned")
	})
}

// setupPushTest creates a common test setup for push notification tests
func setupPushTest(t *testing.T, handlerFunc func(w http.ResponseWriter, r *http.Request)) (*push.Notifier, database.Store, string) {
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	db, _ := dbtestutil.NewDB(t)

	server := httptest.NewServer(http.HandlerFunc(handlerFunc))
	t.Cleanup(server.Close)

	manager, err := push.New(context.Background(), &logger, db)
	require.NoError(t, err)

	return manager, db, server.URL
}
