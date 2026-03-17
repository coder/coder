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
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"tailscale.com/util/singleflight"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const defaultSubscriptionCacheTTL = 3 * time.Minute

// Dispatcher is an interface that can be used to dispatch
// web push notifications to clients such as browsers.
type Dispatcher interface {
	// Dispatch sends a web push notification to all subscriptions
	// for a user. Any notifications that fail to send are silently dropped.
	Dispatch(ctx context.Context, userID uuid.UUID, notification codersdk.WebpushMessage) error
	// Test sends a test web push notificatoin to a subscription to ensure it is valid.
	Test(ctx context.Context, req codersdk.WebpushSubscription) error
	// PublicKey returns the VAPID public key for the webpush dispatcher.
	PublicKey() string
}

// SubscriptionCacheInvalidator is an optional interface that lets local
// subscription mutation handlers invalidate cached subscriptions.
type SubscriptionCacheInvalidator interface {
	InvalidateUser(userID uuid.UUID)
}

type options struct {
	clock                quartz.Clock
	subscriptionCacheTTL time.Duration
}

// Option configures optional behavior for a Webpusher.
type Option func(*options)

// WithClock sets the clock used by the subscription cache. Defaults to a real
// clock when not provided.
func WithClock(clock quartz.Clock) Option {
	return func(o *options) {
		o.clock = clock
	}
}

// WithSubscriptionCacheTTL sets the in-memory subscription cache TTL. Defaults
// to three minutes when not provided or when given a non-positive duration.
func WithSubscriptionCacheTTL(ttl time.Duration) Option {
	return func(o *options) {
		o.subscriptionCacheTTL = ttl
	}
}

// New creates a new Dispatcher to dispatch web push notifications.
//
// This is *not* integrated into the enqueue system unfortunately.
// That's because the notifications system has a enqueue system,
// and push notifications at time of implementation are being used
// for updates inside of a workspace, which we want to be immediate.
//
// See: https://github.com/coder/internal/issues/528
func New(ctx context.Context, log *slog.Logger, db database.Store, vapidSub string, opts ...Option) (Dispatcher, error) {
	cfg := options{
		clock:                quartz.NewReal(),
		subscriptionCacheTTL: defaultSubscriptionCacheTTL,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.clock == nil {
		cfg.clock = quartz.NewReal()
	}
	if cfg.subscriptionCacheTTL <= 0 {
		cfg.subscriptionCacheTTL = defaultSubscriptionCacheTTL
	}

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
		vapidSub:                vapidSub,
		store:                   db,
		log:                     log,
		VAPIDPublicKey:          keys.VapidPublicKey,
		VAPIDPrivateKey:         keys.VapidPrivateKey,
		clock:                   cfg.clock,
		subscriptionCacheTTL:    cfg.subscriptionCacheTTL,
		subscriptionCache:       make(map[uuid.UUID]cachedSubscriptions),
		subscriptionGenerations: make(map[uuid.UUID]uint64),
	}, nil
}

type cachedSubscriptions struct {
	subscriptions []database.WebpushSubscription
	expiresAt     time.Time
}

type Webpusher struct {
	store database.Store
	log   *slog.Logger
	// VAPID allows us to identify the sender of the message.
	// This must be a https:// URL or an email address.
	// Some push services (such as Apple's) require this to be set.
	vapidSub string

	// public and private keys for VAPID. These are used to sign and encrypt
	// the message payload.
	VAPIDPublicKey  string
	VAPIDPrivateKey string

	clock quartz.Clock

	cacheMu                 sync.RWMutex
	subscriptionCache       map[uuid.UUID]cachedSubscriptions
	subscriptionGenerations map[uuid.UUID]uint64
	subscriptionCacheTTL    time.Duration
	subscriptionFetches     singleflight.Group[string, []database.WebpushSubscription]
}

func (n *Webpusher) Dispatch(ctx context.Context, userID uuid.UUID, msg codersdk.WebpushMessage) error {
	subscriptions, err := n.subscriptionsForUser(ctx, userID)
	if err != nil {
		return xerrors.Errorf("get web push subscriptions by user ID: %w", err)
	}
	if len(subscriptions) == 0 {
		return nil
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return xerrors.Errorf("marshal webpush notification: %w", err)
	}

	cleanupSubscriptions := make([]uuid.UUID, 0)
	var mu sync.Mutex
	var eg errgroup.Group
	for _, subscription := range subscriptions {
		eg.Go(func() error {
			// TODO: Implement some retry logic here. For now, this is just a
			// best-effort attempt.
			statusCode, body, err := n.webpushSend(ctx, msgJSON, subscription.Endpoint, webpush.Keys{
				Auth:   subscription.EndpointAuthKey,
				P256dh: subscription.EndpointP256dhKey,
			})
			if err != nil {
				return xerrors.Errorf("send webpush notification: %w", err)
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
		return xerrors.Errorf("send webpush notifications: %w", err)
	}

	if len(cleanupSubscriptions) > 0 {
		// nolint:gocritic // These are known to be invalid subscriptions.
		err = n.store.DeleteWebpushSubscriptions(dbauthz.AsNotifier(ctx), cleanupSubscriptions)
		if err != nil {
			n.log.Error(ctx, "failed to delete stale push subscriptions", slog.Error(err))
		} else {
			n.pruneSubscriptions(userID, cleanupSubscriptions)
		}
	}

	return nil
}

func (n *Webpusher) subscriptionsForUser(ctx context.Context, userID uuid.UUID) ([]database.WebpushSubscription, error) {
	if subscriptions, ok := n.cachedSubscriptions(userID); ok {
		return subscriptions, nil
	}

	subscriptions, err, _ := n.subscriptionFetches.Do(userID.String(), func() ([]database.WebpushSubscription, error) {
		if cached, ok := n.cachedSubscriptions(userID); ok {
			return cached, nil
		}

		generation := n.subscriptionGeneration(userID)
		fetched, err := n.store.GetWebpushSubscriptionsByUserID(ctx, userID)
		if err != nil {
			return nil, err
		}
		n.storeSubscriptions(userID, generation, fetched)
		return slices.Clone(fetched), nil
	})
	if err != nil {
		return nil, err
	}

	return slices.Clone(subscriptions), nil
}

func (n *Webpusher) cachedSubscriptions(userID uuid.UUID) ([]database.WebpushSubscription, bool) {
	n.cacheMu.RLock()
	entry, ok := n.subscriptionCache[userID]
	n.cacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if n.clock.Now().Before(entry.expiresAt) {
		return slices.Clone(entry.subscriptions), true
	}

	n.cacheMu.Lock()
	if current, ok := n.subscriptionCache[userID]; ok && !n.clock.Now().Before(current.expiresAt) {
		delete(n.subscriptionCache, userID)
	}
	n.cacheMu.Unlock()

	return nil, false
}

func (n *Webpusher) subscriptionGeneration(userID uuid.UUID) uint64 {
	n.cacheMu.RLock()
	generation := n.subscriptionGenerations[userID]
	n.cacheMu.RUnlock()
	return generation
}

func (n *Webpusher) storeSubscriptions(userID uuid.UUID, generation uint64, subscriptions []database.WebpushSubscription) {
	n.cacheMu.Lock()
	defer n.cacheMu.Unlock()

	if n.subscriptionGenerations[userID] != generation {
		return
	}

	n.subscriptionCache[userID] = cachedSubscriptions{
		subscriptions: slices.Clone(subscriptions),
		expiresAt:     n.clock.Now().Add(n.subscriptionCacheTTL),
	}
}

func (n *Webpusher) pruneSubscriptions(userID uuid.UUID, staleIDs []uuid.UUID) {
	if len(staleIDs) == 0 {
		return
	}

	stale := make(map[uuid.UUID]struct{}, len(staleIDs))
	for _, id := range staleIDs {
		stale[id] = struct{}{}
	}

	n.cacheMu.Lock()
	defer n.cacheMu.Unlock()

	entry, ok := n.subscriptionCache[userID]
	if !ok {
		return
	}
	if !n.clock.Now().Before(entry.expiresAt) {
		delete(n.subscriptionCache, userID)
		return
	}

	filtered := make([]database.WebpushSubscription, 0, len(entry.subscriptions))
	for _, subscription := range entry.subscriptions {
		if _, shouldDelete := stale[subscription.ID]; shouldDelete {
			continue
		}
		filtered = append(filtered, subscription)
	}
	if len(filtered) == 0 {
		delete(n.subscriptionCache, userID)
		return
	}

	entry.subscriptions = filtered
	n.subscriptionCache[userID] = entry
}

// InvalidateUser clears the cached subscriptions for a user and advances
// its invalidation generation. Local subscribe and unsubscribe handlers call
// this after mutating subscriptions in the same process.
func (n *Webpusher) InvalidateUser(userID uuid.UUID) {
	n.cacheMu.Lock()
	delete(n.subscriptionCache, userID)
	n.subscriptionGenerations[userID]++
	n.cacheMu.Unlock()
	n.subscriptionFetches.Forget(userID.String())
}

func (n *Webpusher) webpushSend(ctx context.Context, msg []byte, endpoint string, keys webpush.Keys) (int, []byte, error) {
	// Copy the message to avoid modifying the original.
	cpy := slices.Clone(msg)
	resp, err := webpush.SendNotificationWithContext(ctx, cpy, &webpush.Subscription{
		Endpoint: endpoint,
		Keys:     keys,
	}, &webpush.Options{
		Subscriber:      n.vapidSub,
		VAPIDPublicKey:  n.VAPIDPublicKey,
		VAPIDPrivateKey: n.VAPIDPrivateKey,
	})
	if err != nil {
		n.log.Error(ctx, "failed to send webpush notification", slog.Error(err), slog.F("endpoint", endpoint))
		return -1, nil, xerrors.Errorf("send webpush notification: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, nil, xerrors.Errorf("read response body: %w", err)
	}

	return resp.StatusCode, body, nil
}

func (n *Webpusher) Test(ctx context.Context, req codersdk.WebpushSubscription) error {
	msgJSON, err := json.Marshal(codersdk.WebpushMessage{
		Title: "It's working!",
		Body:  "You've subscribed to push notifications.",
	})
	if err != nil {
		return xerrors.Errorf("marshal webpush notification: %w", err)
	}
	statusCode, body, err := n.webpushSend(ctx, msgJSON, req.Endpoint, webpush.Keys{
		Auth:   req.AuthKey,
		P256dh: req.P256DHKey,
	})
	if err != nil {
		return xerrors.Errorf("send test webpush notification: %w", err)
	}

	// 200, 201, and 202 are common for successful delivery.
	if statusCode > http.StatusAccepted {
		// It's likely the subscription failed to deliver for some reason.
		return xerrors.Errorf("web push dispatch failed with status code %d: %s", statusCode, string(body))
	}

	return nil
}

// PublicKey returns the VAPID public key for the webpush dispatcher.
// Clients need this, so it's exposed via the BuildInfo endpoint.
func (n *Webpusher) PublicKey() string {
	return n.VAPIDPublicKey
}

// NoopWebpusher is a Dispatcher that does nothing except return an error.
// This is returned when web push notifications are disabled, or if there was an
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
