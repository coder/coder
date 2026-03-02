package coderd_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	// These are valid keys for a web push subscription.
	// DO NOT REUSE THESE IN ANY REAL CODE.
	validEndpointAuthKey   = "zqbxT6JKstKSY9JKibZLSQ=="
	validEndpointP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgTk="
)

func TestWebpushSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	client := coderdtest.New(t, &coderdtest.Options{})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	_, anotherMember := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	handlerCalled := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		handlerCalled <- true
	}))
	defer server.Close()

	err := memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  server.URL,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.NoError(t, err, "create webpush subscription")
	require.True(t, <-handlerCalled, "handler should have been called")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message")
	require.True(t, <-handlerCalled, "handler should have been called again")

	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: server.URL,
	})
	require.NoError(t, err, "delete webpush subscription")

	// Deleting the subscription for a non-existent endpoint should return a 404
	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: server.URL,
	})
	var sdkError *codersdk.Error
	require.Error(t, err)
	require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
	require.Equal(t, http.StatusNotFound, sdkError.StatusCode())

	// Creating a subscription for another user should not be allowed.
	err = memberClient.PostWebpushSubscription(ctx, anotherMember.ID.String(), codersdk.WebpushSubscription{
		Endpoint:  server.URL,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.Error(t, err, "create webpush subscription for another user")

	// Deleting a subscription for another user should not be allowed.
	err = memberClient.DeleteWebpushSubscription(ctx, anotherMember.ID.String(), codersdk.DeleteWebpushSubscription{
		Endpoint: server.URL,
	})
	require.Error(t, err, "delete webpush subscription for another user")
}

// testWebpushErrorStore wraps a real database.Store and allows injecting
// errors into GetWebpushSubscriptionsByUserID.
type testWebpushErrorStore struct {
	database.Store
	getWebpushSubscriptionsErr atomic.Pointer[error]
}

func (s *testWebpushErrorStore) GetWebpushSubscriptionsByUserID(ctx context.Context, userID uuid.UUID) ([]database.WebpushSubscription, error) {
	if err := s.getWebpushSubscriptionsErr.Load(); err != nil {
		return nil, *err
	}
	return s.Store.GetWebpushSubscriptionsByUserID(ctx, userID)
}

func TestDeleteWebpushSubscription(t *testing.T) {
	t.Parallel()

	t.Run("database error returns 500", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)

		store, ps := dbtestutil.NewDB(t)
		wrappedStore := &testWebpushErrorStore{Store: store}

		client := coderdtest.New(t, &coderdtest.Options{
			Database: wrappedStore,
			Pubsub:   ps,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		// Inject a database error into
		// GetWebpushSubscriptionsByUserID. The handler should
		// return 500, not mask the error as 404.
		dbErr := xerrors.New("database is unavailable")
		wrappedStore.getWebpushSubscriptionsErr.Store(&dbErr)

		err := memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
			Endpoint: "https://push.example.com/test",
		})
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusInternalServerError, sdkError.StatusCode(), "database errors should return 500, not be masked as 404")
	})
}
