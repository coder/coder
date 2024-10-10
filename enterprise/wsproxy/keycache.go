package wsproxy

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// latestSequence is a special sequence number that represents the latest key.
	latestSequence = -1
	// refreshInterval is the interval at which the key cache will refresh.
	refreshInterval = time.Minute * 10
)

type Fetcher interface {
	Fetch(ctx context.Context) ([]codersdk.CryptoKey, error)
}

type CryptoKeyCache struct {
	Clock         quartz.Clock
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
	fetcher       Fetcher
	logger        slog.Logger

	mu        sync.Mutex
	keys      map[int32]codersdk.CryptoKey
	lastFetch time.Time
	refresher *quartz.Timer
	fetching  bool
	closed    bool
	cond      *sync.Cond
}

func NewCryptoKeyCache(ctx context.Context, log slog.Logger, client Fetcher, opts ...func(*CryptoKeyCache)) (*CryptoKeyCache, error) {
	cache := &CryptoKeyCache{
		Clock:   quartz.NewReal(),
		logger:  log,
		fetcher: client,
	}

	for _, opt := range opts {
		opt(cache)
	}

	cache.cond = sync.NewCond(&cache.mu)
	cache.refreshCtx, cache.refreshCancel = context.WithCancel(ctx)
	cache.refresher = cache.Clock.AfterFunc(refreshInterval, cache.refresh)

	keys, err := cache.cryptoKeys(ctx)
	if err != nil {
		cache.refreshCancel()
		return nil, xerrors.Errorf("initial fetch: %w", err)
	}
	cache.keys = keys

	return cache, nil
}

func (k *CryptoKeyCache) Signing(ctx context.Context) (codersdk.CryptoKey, error) {
	return k.cryptoKey(ctx, latestSequence)
}

func (k *CryptoKeyCache) Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	return k.cryptoKey(ctx, sequence)
}

func (k *CryptoKeyCache) cryptoKey(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	var key codersdk.CryptoKey
	var ok bool
	for key, ok = k.key(sequence); !ok && k.fetching && !k.closed; {
		k.cond.Wait()
	}

	if k.closed {
		return codersdk.CryptoKey{}, cryptokeys.ErrClosed
	}

	if ok {
		return checkKey(key, sequence, k.Clock.Now())
	}

	k.fetching = true
	k.mu.Unlock()

	keys, err := k.cryptoKeys(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("get keys: %w", err)
	}

	k.mu.Lock()
	k.lastFetch = k.Clock.Now()
	k.refresher.Reset(refreshInterval)
	k.keys = keys
	k.fetching = false
	k.cond.Broadcast()

	key, ok = k.key(sequence)
	if !ok {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyNotFound
	}

	return checkKey(key, sequence, k.Clock.Now())
}

func (k *CryptoKeyCache) key(sequence int32) (codersdk.CryptoKey, bool) {
	if sequence == latestSequence {
		return k.keys[latestSequence], k.keys[latestSequence].CanSign(k.Clock.Now())
	}

	key, ok := k.keys[sequence]
	return key, ok
}

func checkKey(key codersdk.CryptoKey, sequence int32, now time.Time) (codersdk.CryptoKey, error) {
	if sequence == latestSequence {
		if !key.CanSign(now) {
			return codersdk.CryptoKey{}, cryptokeys.ErrKeyInvalid
		}
		return key, nil
	}

	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, cryptokeys.ErrKeyInvalid
	}

	return key, nil
}

// refresh fetches the keys from the control plane and updates the cache.
func (k *CryptoKeyCache) refresh() {
	now := k.Clock.Now("CryptoKeyCache", "refresh")
	k.mu.Lock()

	if k.closed {
		k.mu.Unlock()
		return
	}

	// If something's already fetching, we don't need to do anything.
	if k.fetching {
		k.mu.Unlock()
		return
	}

	// There's a window we must account for where the timer fires while a fetch
	// is ongoing but prior to the timer getting reset. In this case we want to
	// avoid double fetching.
	if now.Sub(k.lastFetch) < refreshInterval {
		k.mu.Unlock()
		return
	}

	k.fetching = true

	k.mu.Unlock()
	keys, err := k.cryptoKeys(k.refreshCtx)
	if err != nil {
		k.logger.Error(k.refreshCtx, "fetch crypto keys", slog.Error(err))
		return
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	k.lastFetch = k.Clock.Now()
	k.refresher.Reset(refreshInterval)
	k.keys = keys
	k.fetching = false
	k.cond.Broadcast()
}

// cryptoKeys queries the control plane for the crypto keys.
// Outside of initialization, this should only be called by fetch.
func (k *CryptoKeyCache) cryptoKeys(ctx context.Context) (map[int32]codersdk.CryptoKey, error) {
	keys, err := k.fetcher.Fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("crypto keys: %w", err)
	}
	cache := toKeyMap(keys, k.Clock.Now())
	return cache, nil
}

func toKeyMap(keys []codersdk.CryptoKey, now time.Time) map[int32]codersdk.CryptoKey {
	m := make(map[int32]codersdk.CryptoKey)
	var latest codersdk.CryptoKey
	for _, key := range keys {
		m[key.Sequence] = key
		if key.Sequence > latest.Sequence && key.CanSign(now) {
			m[latestSequence] = key
		}
	}
	return m
}

func (k *CryptoKeyCache) Close() {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return
	}

	k.closed = true
	k.refreshCancel()
	k.refresher.Stop()
	k.cond.Broadcast()
}
