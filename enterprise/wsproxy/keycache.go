package wsproxy

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
	"github.com/coder/quartz"
)

var _ cryptokeys.Keycache = &CryptoKeyCache{}

type CryptoKeyCache struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *wsproxysdk.Client
	logger slog.Logger
	Clock  quartz.Clock

	keysMu sync.RWMutex
	keys   map[int32]codersdk.CryptoKey
	latest codersdk.CryptoKey
	closed bool
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
	cache.ctx, cache.cancel = context.WithCancel(ctx)

	go cache.refresh()

	return cache, nil
}

func (k *CryptoKeyCache) Signing(ctx context.Context) (codersdk.CryptoKey, error) {
	k.keysMu.RLock()

	if k.closed {
		k.keysMu.RUnlock()
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	latest := k.latest
	k.keysMu.RUnlock()

	now := k.Clock.Now()
	if latest.CanSign(now) {
		return latest, nil
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.closed {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	if k.latest.CanSign(now) {
		return k.latest, nil
	}

	var err error
	k.keys, k.latest, err = k.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	if !k.latest.CanSign(now) {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	return k.latest, nil
}

func (k *CryptoKeyCache) Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	now := k.Clock.Now()
	k.keysMu.RLock()
	if k.closed {
		k.keysMu.RUnlock()
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	key, ok := k.keys[sequence]
	k.keysMu.RUnlock()
	if ok {
		return validKey(key, now)
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.closed {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	key, ok = k.keys[sequence]
	if ok {
		return validKey(key, now)
	}

	var err error
	k.keys, k.latest, err = k.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	key, ok = k.keys[sequence]
	if !ok {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	return validKey(key, now)
}

func (k *CryptoKeyCache) refresh() {
	k.Clock.TickerFunc(k.ctx, time.Minute*10, func() error {
		kmap, latest, err := k.fetch(k.ctx)
		if err != nil {
			k.logger.Error(k.ctx, "failed to fetch crypto keys", slog.Error(err))
			return nil
		}

		k.keysMu.Lock()
		defer k.keysMu.Unlock()
		k.keys = kmap
		k.latest = latest
		return nil
	})
}

func (k *CryptoKeyCache) fetch(ctx context.Context) (map[int32]codersdk.CryptoKey, codersdk.CryptoKey, error) {
	keys, err := k.client.CryptoKeys(ctx)
	if err != nil {
		return nil, codersdk.CryptoKey{}, xerrors.Errorf("get security keys: %w", err)
	}

	kmap, latest := toKeyMap(keys.CryptoKeys, k.Clock.Now())
	return kmap, latest, nil
}

func toKeyMap(keys []codersdk.CryptoKey, now time.Time) (map[int32]codersdk.CryptoKey, codersdk.CryptoKey) {
	m := make(map[int32]codersdk.CryptoKey)
	var latest codersdk.CryptoKey
	for _, key := range keys {
		m[key.Sequence] = key
		if key.Sequence > latest.Sequence && key.CanSign(now) {
			latest = key
		}
	}
	return m, latest
}

func validKey(key codersdk.CryptoKey, now time.Time) (codersdk.CryptoKey, error) {
	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyInvalid
	}

	return key, nil
}

func (k *CryptoKeyCache) Close() {
	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.closed {
		return
	}

	k.cancel()
	k.closed = true
}
