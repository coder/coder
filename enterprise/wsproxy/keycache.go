package wsproxy

import (
	"context"
	"sync"
	"sync/atomic"
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
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
	client        *wsproxysdk.Client
	logger        slog.Logger
	Clock         quartz.Clock

	keysMu    sync.RWMutex
	keys      map[int32]codersdk.CryptoKey
	latest    codersdk.CryptoKey
	fetchLock sync.RWMutex
	lastFetch time.Time
	refresher *quartz.Timer
	closed    atomic.Bool
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
	cache.refresher = cache.Clock.AfterFunc(time.Minute*10, cache.refresh)

	return cache, nil
}

func (k *CryptoKeyCache) Signing(ctx context.Context) (codersdk.CryptoKey, error) {
	if k.isClosed() {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	k.keysMu.RLock()
	latest := k.latest
	k.keysMu.RUnlock()

	now := k.Clock.Now()
	if latest.CanSign(now) {
		return latest, nil
	}

	k.fetchLock.Lock()
	defer k.fetchLock.Unlock()

	if k.isClosed() {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	k.keysMu.RLock()
	if k.latest.CanSign(now) {
		k.keysMu.RUnlock()
		return k.latest, nil
	}

	_, latest, err := k.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	return latest, nil
}

func (k *CryptoKeyCache) Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	now := k.Clock.Now()
	k.keysMu.RLock()
	if k.isClosed() {
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

	if k.isClosed() {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	key, ok = k.keys[sequence]
	if ok {
		return validKey(key, now)
	}

	keys, _, err := k.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	key, ok = keys[sequence]
	if !ok {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	return validKey(key, now)
}

func (k *CryptoKeyCache) refresh() {
	if k.isClosed() {
		return
	}

	k.keysMu.RLock()
	if k.Clock.Now().Sub(k.lastFetch) < time.Minute*10 {
		k.keysMu.Unlock()
		return
	}

	k.fetchLock.Lock()
	defer k.fetchLock.Unlock()

	_, _, err := k.fetch(k.refreshCtx)
	if err != nil {
		k.logger.Error(k.refreshCtx, "fetch crypto keys", slog.Error(err))
		return
	}
}

func (k *CryptoKeyCache) fetch(ctx context.Context) (map[int32]codersdk.CryptoKey, codersdk.CryptoKey, error) {

	keys, err := k.client.CryptoKeys(ctx)
	if err != nil {
		return nil, codersdk.CryptoKey{}, xerrors.Errorf("get security keys: %w", err)
	}

	if len(keys.CryptoKeys) == 0 {
		return nil, codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	now := k.Clock.Now()
	kmap, latest := toKeyMap(keys.CryptoKeys, now)
	if !latest.CanSign(now) {
		return nil, codersdk.CryptoKey{}, cryptokeys.ErrKeyInvalid
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	k.lastFetch = k.Clock.Now()
	k.refresher.Reset(time.Minute * 10)
	k.keys, k.latest = kmap, latest

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

func (k *CryptoKeyCache) isClosed() bool {
	return k.closed.Load()
}

func (k *CryptoKeyCache) Close() {
	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.isClosed() {
		return
	}

	k.refreshCancel()
	k.closed.Store(true)
}
