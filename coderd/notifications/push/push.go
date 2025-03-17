package push

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// New creates a new push manager to dispatch push notifications.
//
// This is *not* integrated into the enqueue system unfortunately.
// That's because the notifications system has a enqueue system,
// and push notifications at time of implementation are being used
// for updates inside of a workspace, which we want to be immediate.
//
// There should be refactor of the core abstraction to merge dispatch
// and queue, and then we can integrate this.
func New(ctx context.Context, log *slog.Logger, db database.Store) (*Notifier, error) {
	keys, err := db.GetNotificationVAPIDKeys(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get notification vapid keys: %w", err)
		}
	}
	if keys.VapidPublicKey == "" || keys.VapidPrivateKey == "" {
		privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, xerrors.Errorf("generate vapid keys: %w", err)
		}
		err = db.UpsertNotificationVAPIDKeys(ctx, database.UpsertNotificationVAPIDKeysParams{
			VapidPublicKey:  publicKey,
			VapidPrivateKey: privateKey,
		})
		if err != nil {
			return nil, xerrors.Errorf("upsert notification vapid keys: %w", err)
		}
		keys.VapidPublicKey = publicKey
		keys.VapidPrivateKey = privateKey
	}

	return &Notifier{
		store:           db,
		log:             log,
		VAPIDPublicKey:  keys.VapidPublicKey,
		VAPIDPrivateKey: keys.VapidPrivateKey,
	}, nil
}

type Notifier struct {
	store database.Store
	log   *slog.Logger

	VAPIDPublicKey  string
	VAPIDPrivateKey string
}

func (n *Notifier) Dispatch(ctx context.Context, userID uuid.UUID, notification codersdk.PushNotification) error {
	subscriptions, err := n.store.GetNotificationPushSubscriptionsByUserID(ctx, userID)
	if err != nil {
		return xerrors.Errorf("get notification push subscriptions by user ID: %w", err)
	}
	if len(subscriptions) == 0 {
		return nil
	}

	notificationJSON, err := json.Marshal(notification)
	if err != nil {
		return xerrors.Errorf("marshal notification: %w", err)
	}

	cleanupSubscriptions := make([]uuid.UUID, 0)
	var mu sync.Mutex
	var eg errgroup.Group
	for _, subscription := range subscriptions {
		subscription := subscription
		eg.Go(func() error {
			n.log.Debug(ctx, "dispatching via push", slog.F("subscription", subscription.Endpoint))

			resp, err := webpush.SendNotificationWithContext(ctx, notificationJSON, &webpush.Subscription{
				Endpoint: subscription.Endpoint,
				Keys: webpush.Keys{
					Auth:   subscription.EndpointAuthKey,
					P256dh: subscription.EndpointP256dhKey,
				},
			}, &webpush.Options{
				VAPIDPublicKey:  n.VAPIDPublicKey,
				VAPIDPrivateKey: n.VAPIDPrivateKey,
			})
			if err != nil {
				return xerrors.Errorf("send notification: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusGone {
				// The subscription is no longer valid, remove it.
				mu.Lock()
				cleanupSubscriptions = append(cleanupSubscriptions, subscription.ID)
				mu.Unlock()
				return nil
			}

			// 200, 201, and 202 are common for successful delivery.
			if resp.StatusCode > http.StatusAccepted {
				// It's likely the subscription failed to deliver for some reason.
				body, _ := io.ReadAll(resp.Body)
				return xerrors.Errorf("push notification failed with status code %d: %s", resp.StatusCode, string(body))
			}

			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return xerrors.Errorf("send push notifications: %w", err)
	}

	if len(cleanupSubscriptions) > 0 {
		// nolint:gocritic // These are known to be invalid subscriptions.
		err = n.store.DeleteNotificationPushSubscriptions(dbauthz.AsSystemRestricted(ctx), cleanupSubscriptions)
		if err != nil {
			n.log.Error(ctx, "failed to delete stale push subscriptions", slog.Error(err))
		}
	}

	return nil
}
