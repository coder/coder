package cryptokeys

import (
	"context"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	// latestSequence is a special sequence number that represents the latest key.
	latestSequence = -1
	// refreshInterval is the interval at which the key cache will refresh.
	refreshInterval = time.Minute * 10
)

type DBFetcher struct {
	DB      database.Store
	Feature database.CryptoKeyFeature
}

func (d *DBFetcher) Fetch(ctx context.Context) ([]codersdk.CryptoKey, error) {
	keys, err := d.DB.GetCryptoKeysByFeature(ctx, d.Feature)
	if err != nil {
		return nil, xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	return db2sdk.CryptoKeys(keys), nil
}

// CryptoKeyCache implements Keycache for callers with access to the database.
type CryptoKeyCache struct {
	clock         quartz.Clock
	refreshCtx    context.Context
	refreshCancel context.CancelFunc
	fetcher       Fetcher
	logger        slog.Logger
	feature       database.CryptoKeyFeature

	mu        sync.Mutex
	keys      map[int32]codersdk.CryptoKey
	lastFetch time.Time
	refresher *quartz.Timer
	fetching  bool
	closed    bool
	cond      *sync.Cond
}

type DBCacheOption func(*CryptoKeyCache)

func WithDBCacheClock(clock quartz.Clock) DBCacheOption {
	return func(d *CryptoKeyCache) {
		d.clock = clock
	}
}

// NewSigningCache creates a new DBCache. Close should be called to
// release resources associated with its internal timer.
func NewSigningCache(ctx context.Context, logger slog.Logger, fetcher Fetcher, feature database.CryptoKeyFeature, opts ...func(*CryptoKeyCache)) (SigningKeycache, error) {
	if !isSigningKeyFeature(feature) {
		return nil, ErrInvalidFeature
	}

	return newDBCache(ctx, logger, fetcher, feature, opts...)
}

func NewEncryptionCache(ctx context.Context, logger slog.Logger, fetcher Fetcher, feature database.CryptoKeyFeature, opts ...func(*CryptoKeyCache)) (EncryptionKeycache, error) {
	if !isEncryptionKeyFeature(feature) {
		return nil, ErrInvalidFeature
	}

	return newDBCache(ctx, logger, fetcher, feature, opts...)
}

func newDBCache(ctx context.Context, logger slog.Logger, fetcher Fetcher, feature database.CryptoKeyFeature, opts ...func(*CryptoKeyCache)) (*CryptoKeyCache, error) {
	cache := &CryptoKeyCache{
		clock:   quartz.NewReal(),
		logger:  logger,
		fetcher: fetcher,
		feature: feature,
	}

	for _, opt := range opts {
		opt(cache)
	}

	cache.cond = sync.NewCond(&cache.mu)
	cache.refreshCtx, cache.refreshCancel = context.WithCancel(ctx)
	cache.refresher = cache.clock.AfterFunc(refreshInterval, cache.refresh)

	keys, err := cache.cryptoKeys(ctx)
	if err != nil {
		cache.refreshCancel()
		return nil, xerrors.Errorf("initial fetch: %w", err)
	}
	cache.keys = keys
	return cache, nil
}

func (d *CryptoKeyCache) EncryptingKey(ctx context.Context) (string, interface{}, error) {
	if !isEncryptionKeyFeature(d.feature) {
		return "", nil, ErrInvalidFeature
	}

	key, err := d.cryptoKey(ctx, latestSequence)
	if err != nil {
		return "", nil, xerrors.Errorf("crypto key: %w", err)
	}

	secret, err := hex.DecodeString(key.Secret)
	if err != nil {
		return "", nil, xerrors.Errorf("decode key: %w", err)
	}

	return strconv.FormatInt(int64(key.Sequence), 10), secret, nil
}

func (d *CryptoKeyCache) DecryptingKey(ctx context.Context, id string) (interface{}, error) {
	if !isEncryptionKeyFeature(d.feature) {
		return nil, ErrInvalidFeature
	}

	i, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, xerrors.Errorf("parse id: %w", err)
	}

	key, err := d.cryptoKey(ctx, int32(i))
	if err != nil {
		return nil, xerrors.Errorf("crypto key: %w", err)
	}

	secret, err := hex.DecodeString(key.Secret)
	if err != nil {
		return nil, xerrors.Errorf("decode key: %w", err)
	}

	return secret, nil
}

func (d *CryptoKeyCache) SigningKey(ctx context.Context) (string, interface{}, error) {
	if !isSigningKeyFeature(d.feature) {
		return "", nil, ErrInvalidFeature
	}

	key, err := d.cryptoKey(ctx, latestSequence)
	if err != nil {
		return "", nil, xerrors.Errorf("crypto key: %w", err)
	}

	return strconv.FormatInt(int64(key.Sequence), 10), key.Secret, nil
}

func (d *CryptoKeyCache) VerifyingKey(ctx context.Context, sequence string) (interface{}, error) {
	if !isSigningKeyFeature(d.feature) {
		return nil, ErrInvalidFeature
	}

	i, err := strconv.ParseInt(sequence, 10, 64)
	if err != nil {
		return nil, xerrors.Errorf("parse id: %w", err)
	}

	key, err := d.cryptoKey(ctx, int32(i))
	if err != nil {
		return nil, xerrors.Errorf("crypto key: %w", err)
	}

	return key.Secret, nil
}

func isEncryptionKeyFeature(feature database.CryptoKeyFeature) bool {
	return feature == database.CryptoKeyFeatureWorkspaceApps
}

func isSigningKeyFeature(feature database.CryptoKeyFeature) bool {
	switch feature {
	case database.CryptoKeyFeatureTailnetResume, database.CryptoKeyFeatureOidcConvert:
		return true
	default:
		return false
	}
}

func idSecret(k database.CryptoKey) (string, interface{}, error) {
	key, err := k.DecodeString()
	if err != nil {
		return "", nil, xerrors.Errorf("decode key: %w", err)
	}

	return strconv.FormatInt(int64(k.Sequence), 10), key, nil
}

func (k *CryptoKeyCache) cryptoKey(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return codersdk.CryptoKey{}, ErrClosed
	}

	var key codersdk.CryptoKey
	var ok bool
	for key, ok = k.key(sequence); !ok && k.fetching && !k.closed; {
		k.cond.Wait()
	}

	if k.closed {
		return codersdk.CryptoKey{}, ErrClosed
	}

	if ok {
		return checkKey(key, sequence, k.clock.Now())
	}

	k.fetching = true
	k.mu.Unlock()

	keys, err := k.cryptoKeys(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("get keys: %w", err)
	}

	k.mu.Lock()
	k.lastFetch = k.clock.Now()
	k.refresher.Reset(refreshInterval)
	k.keys = keys
	k.fetching = false
	k.cond.Broadcast()

	key, ok = k.key(sequence)
	if !ok {
		return codersdk.CryptoKey{}, ErrKeyNotFound
	}

	return checkKey(key, sequence, k.clock.Now())
}

func (k *CryptoKeyCache) key(sequence int32) (codersdk.CryptoKey, bool) {
	if sequence == latestSequence {
		return k.keys[latestSequence], k.keys[latestSequence].CanSign(k.clock.Now())
	}

	key, ok := k.keys[sequence]
	return key, ok
}

func checkKey(key codersdk.CryptoKey, sequence int32, now time.Time) (codersdk.CryptoKey, error) {
	if sequence == latestSequence {
		if !key.CanSign(now) {
			return codersdk.CryptoKey{}, ErrKeyInvalid
		}
		return key, nil
	}

	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	return key, nil
}

// refresh fetches the keys and updates the cache.
func (k *CryptoKeyCache) refresh() {
	now := k.clock.Now("CryptoKeyCache", "refresh")
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

	k.lastFetch = k.clock.Now()
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
	cache := toKeyMap(keys, k.clock.Now())
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

func (k *CryptoKeyCache) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return nil
	}

	k.closed = true
	k.refreshCancel()
	k.refresher.Stop()
	k.cond.Broadcast()

	return nil
}
