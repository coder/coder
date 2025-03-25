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

// NotificationDispatcher is an interface that can be used to dispatch
// push notifications.
type NotificationDispatcher interface {
	Dispatch(ctx context.Context, userID uuid.UUID, notification codersdk.PushNotification) error
	PublicKey() string
	PrivateKey() string
}

// New creates a new push manager to dispatch push notifications.
//
// This is *not* integrated into the enqueue system unfortunately.
// That's because the notifications system has a enqueue system,
// and push notifications at time of implementation are being used
// for updates inside of a workspace, which we want to be immediate.
//
// See: https://github.com/coder/internal/issues/528
func New(ctx context.Context, log *slog.Logger, db database.Store) (NotificationDispatcher, error) {
	keys, err := db.GetNotificationVAPIDKeys(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get notification vapid keys: %w", err)
		}
	}
	if keys.VapidPublicKey == "" || keys.VapidPrivateKey == "" {
		// Generate new VAPID keys. This also deletes all existing push
		// subscriptions as part of the transaction, as they are no longer
		// valid.
		newPrivateKey, newPublicKey, err := RegenerateVAPIDKeys(ctx, db)
		if err != nil {
			return nil, xerrors.Errorf("regenerate vapid keys: %w", err)
		}

		keys.VapidPublicKey = newPublicKey
		keys.VapidPrivateKey = newPrivateKey
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
		err = n.store.DeleteNotificationPushSubscriptions(dbauthz.AsNotifier(ctx), cleanupSubscriptions)
		if err != nil {
			n.log.Error(ctx, "failed to delete stale push subscriptions", slog.Error(err))
		}
	}

	return nil
}

func (n *Notifier) PublicKey() string {
	return n.VAPIDPublicKey
}

func (n *Notifier) PrivateKey() string {
	return n.VAPIDPrivateKey
}

// NoopNotifier is a Notifier that does nothing except return an error.
// This is returned when push notifications are disabled, or if there was an
// error generating the VAPID keys.
type NoopNotifier struct {
	Msg string
}

func (n *NoopNotifier) Dispatch(context.Context, uuid.UUID, codersdk.PushNotification) error {
	return xerrors.New(n.Msg)
}

func (*NoopNotifier) PublicKey() string {
	return ""
}

func (*NoopNotifier) PrivateKey() string {
	return ""
}

// RegenerateVAPIDKeys regenerates the VAPID keys and deletes all existing
// push subscriptions as part of the transaction, as they are no longer valid.
func RegenerateVAPIDKeys(ctx context.Context, db database.Store) (newPrivateKey string, newPublicKey string, err error) {
	newPrivateKey, newPublicKey, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", xerrors.Errorf("generate new vapid keypair: %w", err)
	}

	if txErr := db.InTx(func(tx database.Store) error {
		if err := tx.DeleteAllNotificationPushSubscriptions(ctx); err != nil {
			return xerrors.Errorf("delete all notification push subscriptions: %w", err)
		}
		if err := tx.UpsertNotificationVAPIDKeys(ctx, database.UpsertNotificationVAPIDKeysParams{
			VapidPrivateKey: newPrivateKey,
			VapidPublicKey:  newPublicKey,
		}); err != nil {
			return xerrors.Errorf("upsert notification vapid key: %w", err)
		}
		return nil
	}, nil); txErr != nil {
		return "", "", xerrors.Errorf("regenerate vapid keypair: %w", txErr)
	}

	return newPrivateKey, newPublicKey, nil
}
