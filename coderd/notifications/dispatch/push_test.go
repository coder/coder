package dispatch_test

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
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/codersdk"
)

const validEndpointAuthKey = "zqbxT6JKstKSY9JKibZLSQ=="
const validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="

func TestPush(t *testing.T) {
	t.Parallel()

	t.Run("SuccessfulDelivery", func(t *testing.T) {
		t.Parallel()
		handler, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		userID := uuid.New()
		store.subscriptions = []database.NotificationPushSubscription{
			{
				ID:                uuid.New(),
				UserID:            userID,
				Endpoint:          serverURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
		}
		payload := types.MessagePayload{
			UserID: userID.String(),
			Actions: []types.TemplateAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
			Labels: map[string]string{
				"icon": "workspace",
			},
		}
		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.NoError(t, err)
		assert.False(t, retry)
		assert.Empty(t, store.deletedIDs, "No subscriptions should be deleted on successful delivery")
	})

	t.Run("ExpiredSubscription", func(t *testing.T) {
		t.Parallel()
		handler, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusGone)
		})
		userID := uuid.New()
		subID := uuid.New()
		store.subscriptions = []database.NotificationPushSubscription{
			{
				ID:                subID,
				UserID:            userID,
				Endpoint:          serverURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
		}
		payload := types.MessagePayload{
			UserID: userID.String(),
		}
		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.NoError(t, err)
		assert.False(t, retry)
		assert.Contains(t, store.deletedIDs, subID, "Expired subscription should be deleted")
	})

	t.Run("FailedDelivery", func(t *testing.T) {
		t.Parallel()
		handler, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))
		})
		userID := uuid.New()
		store.subscriptions = []database.NotificationPushSubscription{
			{
				ID:                uuid.New(),
				UserID:            userID,
				Endpoint:          serverURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
		}
		payload := types.MessagePayload{
			UserID: userID.String(),
		}
		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.Error(t, err)
		assert.False(t, retry)
		assert.Contains(t, err.Error(), "Invalid request")
		assert.Empty(t, store.deletedIDs, "No subscriptions should be deleted on failed delivery")
	})

	t.Run("InvalidUserID", func(t *testing.T) {
		t.Parallel()
		handler, _, _ := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		payload := types.MessagePayload{
			UserID: "invalid-uuid",
		}
		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.Error(t, err)
		assert.False(t, retry)
		assert.Contains(t, err.Error(), "parse user ID")
	})

	t.Run("MultipleSubscriptions", func(t *testing.T) {
		t.Parallel()

		var okEndpointCalled bool
		var goneEndpointCalled bool
		handler, store, serverOKURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
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

		store.subscriptions = []database.NotificationPushSubscription{
			{
				ID:                sub1ID,
				UserID:            userID,
				Endpoint:          serverOKURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
			{
				ID:                sub2ID,
				UserID:            userID,
				Endpoint:          serverGoneURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
		}

		payload := types.MessagePayload{
			UserID: userID.String(),
			Actions: []types.TemplateAction{
				{Label: "View", URL: "https://coder.com/view"},
			},
		}

		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.NoError(t, err)
		assert.False(t, retry)
		assert.True(t, okEndpointCalled, "The valid endpoint should be called")
		assert.True(t, goneEndpointCalled, "The expired endpoint should be called")
		assert.Len(t, store.deletedIDs, 1, "One subscription should be deleted")
		assert.Contains(t, store.deletedIDs, sub2ID, "The expired subscription should be deleted")
		assert.NotContains(t, store.deletedIDs, sub1ID, "The valid subscription should not be deleted")
	})

	t.Run("NotificationPayload", func(t *testing.T) {
		t.Parallel()
		var requestReceived bool
		handler, store, serverURL := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			requestReceived = true
			w.WriteHeader(http.StatusOK)
		})

		userID := uuid.New()
		store.subscriptions = []database.NotificationPushSubscription{
			{
				ID:                uuid.New(),
				UserID:            userID,
				Endpoint:          serverURL,
				EndpointAuthKey:   validEndpointAuthKey,
				EndpointP256dhKey: validEndpointP256dhKey,
			},
		}

		payload := types.MessagePayload{
			UserID: userID.String(),
			Actions: []types.TemplateAction{
				{Label: "View Workspace", URL: "https://coder.com/workspace/123"},
				{Label: "Cancel", URL: "https://coder.com/cancel"},
			},
			Labels: map[string]string{
				"icon": "workspace-icon",
			},
		}

		dispatchFunc, err := handler.Dispatcher(payload, "Test Notification", "This is a test notification body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.NoError(t, err)
		assert.False(t, retry)
		assert.True(t, requestReceived, "The push notification request should have been received by the server")
	})

	t.Run("NoSubscriptions", func(t *testing.T) {
		t.Parallel()
		handler, store, _ := setupPushTest(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Empty the subscriptions
		store.subscriptions = []database.NotificationPushSubscription{}

		userID := uuid.New()
		payload := types.MessagePayload{
			UserID: userID.String(),
		}

		dispatchFunc, err := handler.Dispatcher(payload, "Test Title", "Test Body", nil)
		require.NoError(t, err)

		msgID := uuid.New()
		retry, err := dispatchFunc(context.Background(), msgID)

		require.NoError(t, err)
		assert.False(t, retry)
		assert.Empty(t, store.deletedIDs, "No subscriptions should be deleted")
	})
}

// mockPushStore implements the PushStore interface for testing
type mockPushStore struct {
	subscriptions []database.NotificationPushSubscription
	deletedIDs    []uuid.UUID
}

func (m *mockPushStore) GetNotificationPushSubscriptionsByUserID(_ context.Context, userID uuid.UUID) ([]database.NotificationPushSubscription, error) {
	return m.subscriptions, nil
}

func (m *mockPushStore) DeleteNotificationPushSubscriptions(_ context.Context, subscriptionIDs []uuid.UUID) error {
	m.deletedIDs = subscriptionIDs
	return nil
}

// setupPushTest creates a common test setup for push notification tests
func setupPushTest(t *testing.T, handlerFunc func(w http.ResponseWriter, r *http.Request)) (*dispatch.PushHandler, *mockPushStore, string) {
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	store := &mockPushStore{}

	server := httptest.NewServer(http.HandlerFunc(handlerFunc))
	t.Cleanup(server.Close)

	return dispatch.NewPushHandler(codersdk.NotificationsPushConfig{
		VAPIDPublicKey:  "test-public",
		VAPIDPrivateKey: "test-private",
	}, logger, store), store, server.URL
}
