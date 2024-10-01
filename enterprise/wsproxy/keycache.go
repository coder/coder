package wsproxy

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	"github.com/coder/quartz"
)

type CryptoKeyCache struct {
	client *wsproxysdk.Client
	logger slog.Logger
	Clock  quartz.Clock

	keysMu sync.RWMutex
	keys   map[int32]wsproxysdk.CryptoKey
	latest wsproxysdk.CryptoKey
}

func NewCryptoKeyCache(ctx context.Context, log slog.Logger, client *wsproxysdk.Client, opts ...func(*CryptoKeyCache)) (*CryptoKeyCache, error) {
	cache := &CryptoKeyCache{
		client: client,
		logger: log,
		Clock:  quartz.NewReal(),
	}

	for _, opt := range opts {
		opt(cache)
	}

	m, latest, err := cache.fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("initial fetch: %w", err)
	}
	cache.keys, cache.latest = m, latest

	go cache.refresh(ctx)

	return cache, nil
}

func (k *CryptoKeyCache) Latest(ctx context.Context) (wsproxysdk.CryptoKey, error) {
	k.keysMu.RLock()
	latest := k.latest
	k.keysMu.RUnlock()

	now := k.Clock.Now().UTC()
	if latest.Active(now) {
		return latest, nil
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.latest.Active(now) {
		return k.latest, nil
	}

	var err error
	k.keys, k.latest, err = k.fetch(ctx)
	if err != nil {
		return wsproxysdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	if !k.latest.Active(now) {
		return wsproxysdk.CryptoKey{}, xerrors.Errorf("no active keys found")
	}

	return k.latest, nil
}

func (k *CryptoKeyCache) Version(ctx context.Context, sequence int32) (wsproxysdk.CryptoKey, error) {
	now := k.Clock.Now().UTC()
	k.keysMu.RLock()
	key, ok := k.keys[sequence]
	k.keysMu.RUnlock()
	if ok {
		return validKey(key, now)
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()
	key, ok = k.keys[sequence]
	if ok {
		return validKey(key, now)
	}

	var err error
	k.keys, k.latest, err = k.fetch(ctx)
	if err != nil {
		return wsproxysdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	key, ok = k.keys[sequence]
	if !ok {
		return wsproxysdk.CryptoKey{}, xerrors.Errorf("key %d not found", sequence)
	}

	return validKey(key, now)
}

func (k *CryptoKeyCache) refresh(ctx context.Context) {
	k.Clock.TickerFunc(ctx, time.Minute*10, func() error {
		kmap, latest, err := k.fetch(ctx)
		if err != nil {
			k.logger.Error(ctx, "failed to fetch crypto keys", slog.Error(err))
			return nil
		}

		k.keysMu.Lock()
		defer k.keysMu.Unlock()
		k.keys = kmap
		k.latest = latest
		return nil
	})
}

func (k *CryptoKeyCache) fetch(ctx context.Context) (map[int32]wsproxysdk.CryptoKey, wsproxysdk.CryptoKey, error) {
	keys, err := k.client.CryptoKeys(ctx)
	if err != nil {
		return nil, wsproxysdk.CryptoKey{}, xerrors.Errorf("get security keys: %w", err)
	}

	kmap, latest := toKeyMap(keys.CryptoKeys, k.Clock.Now().UTC())
	return kmap, latest, nil
}

func toKeyMap(keys []wsproxysdk.CryptoKey, now time.Time) (map[int32]wsproxysdk.CryptoKey, wsproxysdk.CryptoKey) {
	m := make(map[int32]wsproxysdk.CryptoKey)
	var latest wsproxysdk.CryptoKey
	for _, key := range keys {
		m[key.Sequence] = key
		if key.Sequence > latest.Sequence && key.Active(now) {
			latest = key
		}
	}
	return m, latest
}

func validKey(key wsproxysdk.CryptoKey, now time.Time) (wsproxysdk.CryptoKey, error) {
	if key.Invalid(now) {
		return wsproxysdk.CryptoKey{}, xerrors.Errorf("key %d is invalid", key.Sequence)
	}

	return key, nil
}
