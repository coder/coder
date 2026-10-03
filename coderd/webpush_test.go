package coderd_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
	require.Equal(t, int32(1), dispatcher.dispatchCalls.Load(), "dispatch should be called even with no subscriptions")

	err = memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  endpoint,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.NoError(t, err, "create webpush subscription")
	require.Equal(t, int32(1), dispatcher.testCalls.Load(), "subscription validation should call dispatcher test once")
	require.Equal(t, 1, dispatcher.invalidateCount(), "subscribing should invalidate the user's cached subscriptions")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message after subscribing")
	require.Equal(t, int32(2), dispatcher.dispatchCalls.Load(), "dispatch should be called after subscribing")

	err = memberClient.DeleteWebpushSubscription(ctx, "me", codersdk.DeleteWebpushSubscription{
		Endpoint: endpoint,
	})
	require.NoError(t, err, "delete webpush subscription")
	require.Equal(t, 2, dispatcher.invalidateCount(), "unsubscribing should invalidate the user's cached subscriptions")

	err = memberClient.PostTestWebpushMessage(ctx)
	require.NoError(t, err, "test webpush message after unsubscribing")
	require.Equal(t, int32(3), dispatcher.dispatchCalls.Load(), "dispatch should be called after unsubscribing")

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

// TestWebpushSubscribeOverwritesKeys verifies that re-subscribing with the
// same endpoint and rotated keys overwrites the existing row in place rather
// than inserting a duplicate. This is the reinstall path: on iOS, deleting
// the PWA from the home screen and reinstalling can yield the same endpoint
// with new p256dh / auth keys, and Coder must replace the stored keys so
// dispatch encrypts with the keys the device can decrypt.
func TestWebpushSubscribeOverwritesKeys(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	store, ps := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		WebpushDispatcher: &testWebpushDispatcher{},
		Database:          store,
		Pubsub:            ps,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

	const endpoint = "https://push.example.com/subscription/reinstall"
	const secondAuthKey = "AnotherAuthKey/yV1FuojuRmHP42=="
	const secondP256dhKey = "BNNL5ZaTfK81qhXOx23+wewhigUeFb632jN6LvRWCFH1ubQr77FE/9qV1FuojuRmHP42zmf34rXgW80OvUVDgABc="

	// First subscribe with the original keys.
	err := memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  endpoint,
		AuthKey:   validEndpointAuthKey,
		P256DHKey: validEndpointP256dhKey,
	})
	require.NoError(t, err, "initial subscribe")

	// Re-subscribe with the same endpoint but rotated keys. This
	// simulates the post-reinstall path on iOS where the browser
	// retains the endpoint but rotates p256dh / auth.
	err = memberClient.PostWebpushSubscription(ctx, "me", codersdk.WebpushSubscription{
		Endpoint:  endpoint,
		AuthKey:   secondAuthKey,
		P256DHKey: secondP256dhKey,
	})
	require.NoError(t, err, "re-subscribe with rotated keys")

	// The second subscribe must replace the keys in place; we should
	// see exactly one row carrying the new keys.
	subs, err := store.GetWebpushSubscriptionsByUserID(dbauthz.AsSystemRestricted(ctx), member.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "re-subscribe should overwrite the row, not append a duplicate")
	require.Equal(t, endpoint, subs[0].Endpoint)
	require.Equal(t, secondAuthKey, subs[0].EndpointAuthKey, "auth key should be the latest one")
	require.Equal(t, secondP256dhKey, subs[0].EndpointP256dhKey, "p256dh key should be the latest one")
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
	require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
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
	testCalls          atomic.Int32
	dispatchCalls      atomic.Int32
	invalidateUserIDs  []uuid.UUID
	invalidateUserLock sync.Mutex
}

func (d *testWebpushDispatcher) Dispatch(_ context.Context, _ uuid.UUID, _ codersdk.WebpushMessage) error {
	d.dispatchCalls.Add(1)
	return nil
}

func (d *testWebpushDispatcher) Test(_ context.Context, _ codersdk.WebpushSubscription) error {
	d.testCalls.Add(1)
	return nil
}

func (*testWebpushDispatcher) PublicKey() string {
	return ""
}

// InvalidateUser implements webpush.SubscriptionCacheInvalidator so the
// handler exercises the cache-invalidation path on subscribe/unsubscribe.
func (d *testWebpushDispatcher) InvalidateUser(userID uuid.UUID) {
	d.invalidateUserLock.Lock()
	defer d.invalidateUserLock.Unlock()
	d.invalidateUserIDs = append(d.invalidateUserIDs, userID)
}

func (d *testWebpushDispatcher) invalidateCount() int {
	d.invalidateUserLock.Lock()
	defer d.invalidateUserLock.Unlock()
	return len(d.invalidateUserIDs)
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
