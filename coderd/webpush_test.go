package coderd_test

import (
	"context"
	"errors"
	"net/http"
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

	dispatcher := &testWebpushDispatcher{}
	client := coderdtest.New(t, &coderdtest.Options{
		WebpushDispatcher: dispatcher,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	_, anotherMember := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	endpoint := "https://push.example.com/subscription/abc123"

	// Seed the dispatcher cache with an empty subscription set. Creating the
	// subscription should invalidate that entry so the next dispatch sees the new
	// subscription immediately.
	err := memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message without a subscription")
	require.Equal(t, int32(1), dispatcher.dispatchCalls.Load(), "test notifications should be dispatched")

	err = memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  endpoint,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.NoError(t, err, "create webpush subscription")
	require.Equal(t, int32(1), dispatcher.testCalls.Load(), "subscription validation should call dispatcher test once")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message after subscribing")
	require.Equal(t, int32(2), dispatcher.dispatchCalls.Load(), "test notifications should be dispatched")

	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: endpoint,
	})
	require.NoError(t, err, "delete webpush subscription")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message after unsubscribing")
	require.Equal(t, int32(3), dispatcher.dispatchCalls.Load(), "test notifications should be dispatched")

	// Deleting the subscription for a non-existent endpoint should return a 404.
	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: endpoint,
	})
	var sdkError *codersdk.Error
	require.Error(t, err)
	require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
	require.Equal(t, http.StatusNotFound, sdkError.StatusCode())

	// Creating a subscription for another user should not be allowed.
	err = memberClient.PostWebpushSubscription(ctx, anotherMember.ID.String(), codersdk.WebpushSubscription{
		Endpoint:  endpoint,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.Error(t, err, "create webpush subscription for another user")

	// Deleting a subscription for another user should not be allowed.
	err = memberClient.DeleteWebpushSubscription(ctx, anotherMember.ID.String(), codersdk.DeleteWebpushSubscription{
		Endpoint: endpoint,
	})
	require.Error(t, err, "delete webpush subscription for another user")
}

func TestWebpushSubscribeRejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	client := coderdtest.New(t, &coderdtest.Options{
		WebpushDispatcher: &testWebpushDispatcher{},
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	err := memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  "http://127.0.0.1:8080/subscription",
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	var sdkError *codersdk.Error
	require.Error(t, err)
	require.True(t, errors.As(err, &sdkError))
	require.Equal(t, http.StatusBadRequest, sdkError.StatusCode())
	require.Contains(t, sdkError.Error(), "endpoint URL scheme must be https")
}

// testWebpushErrorStore wraps a real database.Store and allows injecting
// errors into GetWebpushSubscriptionsByUserID.
type testWebpushErrorStore struct {
	database.Store
	getWebpushSubscriptionsErr atomic.Pointer[error]
}

type testWebpushDispatcher struct {
	testCalls     atomic.Int32
	dispatchCalls atomic.Int32
}

func (d *testWebpushDispatcher) Dispatch(context.Context, uuid.UUID, codersdk.WebpushMessage) error {
	d.dispatchCalls.Add(1)
	return nil
}

func (d *testWebpushDispatcher) Test(context.Context, codersdk.WebpushSubscription) error {
	d.testCalls.Add(1)
	return nil
}

func (*testWebpushDispatcher) PublicKey() string {
	return ""
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
