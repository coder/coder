package webpush

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
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

// Dispatcher is an interface that can be used to dispatch
// push notifications over Web Push.
type Dispatcher interface {
	// Dispatch sends a notification to all subscriptions for a user. Any
	// notifications that fail to send are silently dropped.
	Dispatch(ctx context.Context, userID uuid.UUID, notification codersdk.WebpushMessage) error
	// Test sends a test notification to a subscription to ensure it is valid.
	Test(ctx context.Context, req codersdk.WebpushSubscription) error
	PublicKey() string
}

// New creates a new Dispatcher to dispatch notifications via Web Push.
//
// This is *not* integrated into the enqueue system unfortunately.
// That's because the notifications system has a enqueue system,
// and push notifications at time of implementation are being used
// for updates inside of a workspace, which we want to be immediate.
//
// See: https://github.com/coder/internal/issues/528
func New(ctx context.Context, log *slog.Logger, db database.Store) (Dispatcher, error) {
	keys, err := db.GetWebpushVAPIDKeys(ctx)
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

	return &Webpusher{
		store:           db,
		log:             log,
		VAPIDPublicKey:  keys.VapidPublicKey,
		VAPIDPrivateKey: keys.VapidPrivateKey,
	}, nil
}

type Webpusher struct {
	store database.Store
	log   *slog.Logger

	VAPIDPublicKey  string
	VAPIDPrivateKey string
}

func (n *Webpusher) Dispatch(ctx context.Context, userID uuid.UUID, notification codersdk.WebpushMessage) error {
	subscriptions, err := n.store.GetWebpushSubscriptionsByUserID(ctx, userID)
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
			statusCode, body, err := n.webpushSend(ctx, notificationJSON, subscription.Endpoint, webpush.Keys{
				Auth:   subscription.EndpointAuthKey,
				P256dh: subscription.EndpointP256dhKey,
			})
			if err != nil {
				return xerrors.Errorf("send notification: %w", err)
			}

			if statusCode == http.StatusGone {
				// The subscription is no longer valid, remove it.
				mu.Lock()
				cleanupSubscriptions = append(cleanupSubscriptions, subscription.ID)
				mu.Unlock()
				return nil
			}

			// 200, 201, and 202 are common for successful delivery.
			if statusCode > http.StatusAccepted {
				// It's likely the subscription failed to deliver for some reason.
				return xerrors.Errorf("web push dispatch failed with status code %d: %s", statusCode, string(body))
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
		err = n.store.DeleteWebpushSubscriptions(dbauthz.AsNotifier(ctx), cleanupSubscriptions)
		if err != nil {
			n.log.Error(ctx, "failed to delete stale push subscriptions", slog.Error(err))
		}
	}

	return nil
}

func (n *Webpusher) webpushSend(ctx context.Context, msg []byte, endpoint string, keys webpush.Keys) (int, []byte, error) {
	// Copy the message to avoid modifying the original.
	cpy := slices.Clone(msg)
	resp, err := webpush.SendNotificationWithContext(ctx, cpy, &webpush.Subscription{
		Endpoint: endpoint,
		Keys:     keys,
	}, &webpush.Options{
		VAPIDPublicKey:  n.VAPIDPublicKey,
		VAPIDPrivateKey: n.VAPIDPrivateKey,
	})
	if err != nil {
		return -1, nil, xerrors.Errorf("send notification: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, nil, xerrors.Errorf("read response body: %w", err)
	}

	return resp.StatusCode, body, nil
}

func (n *Webpusher) Test(ctx context.Context, req codersdk.WebpushSubscription) error {
	notificationJSON, err := json.Marshal(codersdk.WebpushMessage{
		Title: "Test",
		Body:  "This is a test notification",
	})
	if err != nil {
		return xerrors.Errorf("marshal notification: %w", err)
	}
	statusCode, body, err := n.webpushSend(ctx, notificationJSON, req.Endpoint, webpush.Keys{
		Auth:   req.AuthKey,
		P256dh: req.P256DHKey,
	})
	if err != nil {
		return xerrors.Errorf("send test notification: %w", err)
	}

	// 200, 201, and 202 are common for successful delivery.
	if statusCode > http.StatusAccepted {
		// It's likely the subscription failed to deliver for some reason.
		return xerrors.Errorf("web push dispatch failed with status code %d: %s", statusCode, string(body))
	}

	return nil
}

func (n *Webpusher) PublicKey() string {
	return n.VAPIDPublicKey
}

// NoopWebpusher is a Dispatcher that does nothing except return an error.
// This is returned when push notifications are disabled, or if there was an
// error generating the VAPID keys.
type NoopWebpusher struct {
	Msg string
}

func (n *NoopWebpusher) Dispatch(context.Context, uuid.UUID, codersdk.WebpushMessage) error {
	return xerrors.New(n.Msg)
}

func (n *NoopWebpusher) Test(context.Context, codersdk.WebpushSubscription) error {
	return xerrors.New(n.Msg)
}

func (*NoopWebpusher) PublicKey() string {
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
		if err := tx.DeleteAllWebpushSubscriptions(ctx); err != nil {
			return xerrors.Errorf("delete all webpush subscriptions: %w", err)
		}
		if err := tx.UpsertWebpushVAPIDKeys(ctx, database.UpsertWebpushVAPIDKeysParams{
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
