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

	cache.refreshCtx, cache.refreshCancel = context.WithCancel(ctx)
	cache.refresher = cache.Clock.AfterFunc(time.Minute*10, cache.refresh)
	m, latest, err := cache.fetchKeys(ctx)
	if err != nil {
		cache.refreshCancel()
		return nil, xerrors.Errorf("initial fetch: %w", err)
	}
	cache.keys, cache.latest = m, latest

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
	latest = k.latest
	k.keysMu.RUnlock()

	now = k.Clock.Now()
	if latest.CanSign(now) {
		return latest, nil
	}

	_, latest, err := k.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	return latest, nil
}

func (k *CryptoKeyCache) Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	if k.isClosed() {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	now := k.Clock.Now()
	k.keysMu.RLock()
	key, ok := k.keys[sequence]
	k.keysMu.RUnlock()
	if ok {
		return validKey(key, now)
	}

	k.fetchLock.Lock()
	defer k.fetchLock.Unlock()

	if k.isClosed() {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	k.keysMu.RLock()
	key, ok = k.keys[sequence]
	k.keysMu.RUnlock()
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

	k.fetchLock.Lock()
	defer k.fetchLock.Unlock()

	if k.isClosed() {
		return
	}

	k.keysMu.RLock()
	lastFetch := k.lastFetch
	k.keysMu.RUnlock()

	// There's a window we must account for where the timer fires while a fetch
	// is ongoing but prior to the timer getting reset. In this case we want to
	// avoid double fetching.
	if k.Clock.Now().Sub(lastFetch) < time.Minute*10 {
		return
	}

	_, _, err := k.fetch(k.refreshCtx)
	if err != nil {
		k.logger.Error(k.refreshCtx, "fetch crypto keys", slog.Error(err))
		return
	}
}

func (k *CryptoKeyCache) fetchKeys(ctx context.Context) (map[int32]codersdk.CryptoKey, codersdk.CryptoKey, error) {
	keys, err := k.client.CryptoKeys(ctx)
	if err != nil {
		return nil, codersdk.CryptoKey{}, xerrors.Errorf("crypto keys: %w", err)
	}
	cache, latest := toKeyMap(keys.CryptoKeys, k.Clock.Now())
	return cache, latest, nil
}

// fetch fetches the keys from the control plane and updates the cache. The fetchMu
// must be held when calling this function to avoid multiple concurrent fetches.
func (k *CryptoKeyCache) fetch(ctx context.Context) (map[int32]codersdk.CryptoKey, codersdk.CryptoKey, error) {
	keys, latest, err := k.fetchKeys(ctx)
	if err != nil {
		return nil, codersdk.CryptoKey{}, xerrors.Errorf("fetch keys: %w", err)
	}

	if len(keys) == 0 {
		return nil, codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	now := k.Clock.Now()
	if !latest.CanSign(now) {
		return nil, codersdk.CryptoKey{}, cryptokeys.ErrKeyInvalid
	}

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	k.lastFetch = k.Clock.Now()
	k.refresher.Reset(time.Minute * 10)
	k.keys, k.latest = keys, latest

	return keys, latest, nil
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
	// The fetch lock must always be held before holding the keys lock
	// otherwise we risk a deadlock.
	k.fetchLock.Lock()
	defer k.fetchLock.Unlock()

	k.keysMu.Lock()
	defer k.keysMu.Unlock()

	if k.isClosed() {
		return
	}

	k.refreshCancel()
	k.closed.Store(true)
}
