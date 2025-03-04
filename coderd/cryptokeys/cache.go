package cryptokeys

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

var (
	ErrKeyNotFound    = xerrors.New("key not found")
	ErrKeyInvalid     = xerrors.New("key is invalid for use")
	ErrClosed         = xerrors.New("closed")
	ErrInvalidFeature = xerrors.New("invalid feature for this operation")
)

type Fetcher interface {
	Fetch(ctx context.Context, feature codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error)
}

type EncryptionKeycache interface {
	// EncryptingKey returns the latest valid key for encrypting payloads. A valid
	// key is one that is both past its start time and before its deletion time.
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
	// DecryptingKey returns the key with the provided id which maps to its sequence
	// number. The key is valid for decryption as long as it is not deleted or past
	// its deletion date. We must allow for keys prior to their start time to
	// account for clock skew between peers (one key may be past its start time on
	// one machine while another is not).
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}

type SigningKeycache interface {
	// SigningKey returns the latest valid key for signing. A valid key is one
	// that is both past its start time and before its deletion time.
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
	// VerifyingKey returns the key with the provided id which should map to its
	// sequence number. The key is valid for verifying as long as it is not deleted
	// or past its deletion date. We must allow for keys prior to their start time
	// to account for clock skew between peers (one key may be past its start time
	// on one machine while another is not).
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
	io.Closer
}

const (
	// latestSequence is a special sequence number that represents the latest key.
	latestSequence = -1
	// refreshInterval is the interval at which the key cache will refresh.
	refreshInterval = time.Minute * 10
)

type DBFetcher struct {
	DB database.Store
}

func (d *DBFetcher) Fetch(ctx context.Context, feature codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error) {
	keys, err := d.DB.GetCryptoKeysByFeature(ctx, database.CryptoKeyFeature(feature))
	if err != nil {
		return nil, xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	return toSDKKeys(keys), nil
}

// cache implements the caching functionality for both signing and encryption keys.
type cache struct {
	ctx     context.Context
	cancel  context.CancelFunc
	clock   quartz.Clock
	fetcher Fetcher
	logger  slog.Logger
	feature codersdk.CryptoKeyFeature

	mu        sync.Mutex
	keys      map[int32]codersdk.CryptoKey
	lastFetch time.Time
	refresher *quartz.Timer
	fetching  bool
	closed    bool
	cond      *sync.Cond
}

type CacheOption func(*cache)

func WithCacheClock(clock quartz.Clock) CacheOption {
	return func(d *cache) {
		d.clock = clock
	}
}

// NewSigningCache instantiates a cache. Close should be called to release resources
// associated with its internal timer.
func NewSigningCache(ctx context.Context, logger slog.Logger, fetcher Fetcher,
	feature codersdk.CryptoKeyFeature, opts ...func(*cache),
) (SigningKeycache, error) {
	if !isSigningKeyFeature(feature) {
		return nil, xerrors.Errorf("invalid feature: %s", feature)
	}
	logger = logger.Named(fmt.Sprintf("%s_signing_keycache", feature))
	return newCache(ctx, logger, fetcher, feature, opts...), nil
}

func NewEncryptionCache(ctx context.Context, logger slog.Logger, fetcher Fetcher,
	feature codersdk.CryptoKeyFeature, opts ...func(*cache),
) (EncryptionKeycache, error) {
	if !isEncryptionKeyFeature(feature) {
		return nil, xerrors.Errorf("invalid feature: %s", feature)
	}
	logger = logger.Named(fmt.Sprintf("%s_encryption_keycache", feature))
	return newCache(ctx, logger, fetcher, feature, opts...), nil
}

func newCache(ctx context.Context, logger slog.Logger, fetcher Fetcher, feature codersdk.CryptoKeyFeature, opts ...func(*cache)) *cache {
	cache := &cache{
		clock:   quartz.NewReal(),
		logger:  logger,
		fetcher: fetcher,
		feature: feature,
	}

	for _, opt := range opts {
		opt(cache)
	}

	cache.cond = sync.NewCond(&cache.mu)
	//nolint:gocritic // We need to be able to read the keys in order to cache them.
	cache.ctx, cache.cancel = context.WithCancel(dbauthz.AsKeyReader(ctx))
	cache.refresher = cache.clock.AfterFunc(refreshInterval, cache.refresh)

	keys, err := cache.cryptoKeys(cache.ctx)
	if err != nil {
		cache.logger.Critical(cache.ctx, "failed initial fetch", slog.Error(err))
	}
	cache.keys = keys
	return cache
}

func (c *cache) EncryptingKey(ctx context.Context) (string, interface{}, error) {
	if !isEncryptionKeyFeature(c.feature) {
		return "", nil, ErrInvalidFeature
	}

	//nolint:gocritic // cache can only read crypto keys.
	ctx = dbauthz.AsKeyReader(ctx)
	return c.cryptoKey(ctx, latestSequence)
}

func (c *cache) DecryptingKey(ctx context.Context, id string) (interface{}, error) {
	if !isEncryptionKeyFeature(c.feature) {
		return nil, ErrInvalidFeature
	}

	seq, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return nil, xerrors.Errorf("parse id: %w", err)
	}

	//nolint:gocritic // cache can only read crypto keys.
	ctx = dbauthz.AsKeyReader(ctx)
	_, secret, err := c.cryptoKey(ctx, int32(seq))
	if err != nil {
		return nil, xerrors.Errorf("crypto key: %w", err)
	}
	return secret, nil
}

func (c *cache) SigningKey(ctx context.Context) (string, interface{}, error) {
	if !isSigningKeyFeature(c.feature) {
		return "", nil, ErrInvalidFeature
	}

	//nolint:gocritic // cache can only read crypto keys.
	ctx = dbauthz.AsKeyReader(ctx)
	return c.cryptoKey(ctx, latestSequence)
}

func (c *cache) VerifyingKey(ctx context.Context, id string) (interface{}, error) {
	if !isSigningKeyFeature(c.feature) {
		return nil, ErrInvalidFeature
	}

	seq, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return nil, xerrors.Errorf("parse id: %w", err)
	}
	//nolint:gocritic // cache can only read crypto keys.
	ctx = dbauthz.AsKeyReader(ctx)
	_, secret, err := c.cryptoKey(ctx, int32(seq))
	if err != nil {
		return nil, xerrors.Errorf("crypto key: %w", err)
	}

	return secret, nil
}

func isEncryptionKeyFeature(feature codersdk.CryptoKeyFeature) bool {
	return feature == codersdk.CryptoKeyFeatureWorkspaceAppsAPIKey
}

func isSigningKeyFeature(feature codersdk.CryptoKeyFeature) bool {
	switch feature {
	case codersdk.CryptoKeyFeatureTailnetResume, codersdk.CryptoKeyFeatureOIDCConvert, codersdk.CryptoKeyFeatureWorkspaceAppsToken:
		return true
	default:
		return false
	}
}

func idSecret(k codersdk.CryptoKey) (string, []byte, error) {
	key, err := hex.DecodeString(k.Secret)
	if err != nil {
		return "", nil, xerrors.Errorf("decode key: %w", err)
	}

	return strconv.FormatInt(int64(k.Sequence), 10), key, nil
}

func (c *cache) cryptoKey(ctx context.Context, sequence int32) (string, []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return "", nil, ErrClosed
	}

	var key codersdk.CryptoKey
	var ok bool
	for key, ok = c.key(sequence); !ok && c.fetching && !c.closed; {
		c.cond.Wait()
	}

	if c.closed {
		return "", nil, ErrClosed
	}

	if ok {
		return checkKey(key, sequence, c.clock.Now())
	}

	c.fetching = true
	c.mu.Unlock()

	keys, err := c.cryptoKeys(ctx)
	if err != nil {
		c.mu.Lock() // Re-lock because of defer.
		return "", nil, xerrors.Errorf("get keys: %w", err)
	}

	c.mu.Lock()
	c.lastFetch = c.clock.Now()
	c.refresher.Reset(refreshInterval)
	c.keys = keys
	c.fetching = false
	c.cond.Broadcast()

	key, ok = c.key(sequence)
	if !ok {
		return "", nil, ErrKeyNotFound
	}

	return checkKey(key, sequence, c.clock.Now())
}

func (c *cache) key(sequence int32) (codersdk.CryptoKey, bool) {
	if sequence == latestSequence {
		return c.keys[latestSequence], c.keys[latestSequence].CanSign(c.clock.Now())
	}

	key, ok := c.keys[sequence]
	return key, ok
}

func checkKey(key codersdk.CryptoKey, sequence int32, now time.Time) (string, []byte, error) {
	if sequence == latestSequence {
		if !key.CanSign(now) {
			return "", nil, ErrKeyInvalid
		}
		return idSecret(key)
	}

	if !key.CanVerify(now) {
		return "", nil, ErrKeyInvalid
	}

	return idSecret(key)
}

// refresh fetches the keys and updates the cache.
func (c *cache) refresh() {
	now := c.clock.Now("CryptoKeyCache", "refresh")
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return
	}

	// If something's already fetching, we don't need to do anything.
	if c.fetching {
		c.mu.Unlock()
		return
	}

	// There's a window we must account for where the timer fires while a fetch
	// is ongoing but prior to the timer getting reset. In this case we want to
	// avoid double fetching.
	if now.Sub(c.lastFetch) < refreshInterval {
		c.mu.Unlock()
		return
	}

	c.fetching = true

	c.mu.Unlock()
	keys, err := c.cryptoKeys(c.ctx)
	if err != nil {
		c.logger.Error(c.ctx, "fetch crypto keys", slog.Error(err))
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastFetch = c.clock.Now()
	c.refresher.Reset(refreshInterval)
	c.keys = keys
	c.fetching = false
	c.cond.Broadcast()
}

// cryptoKeys queries the control plane for the crypto keys.
// Outside of initialization, this should only be called by fetch.
func (c *cache) cryptoKeys(ctx context.Context) (map[int32]codersdk.CryptoKey, error) {
	keys, err := c.fetcher.Fetch(ctx, c.feature)
	if err != nil {
		return nil, xerrors.Errorf("fetch: %w", err)
	}
	cache := toKeyMap(keys, c.clock.Now())
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

func (c *cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	c.cancel()
	c.refresher.Stop()
	c.cond.Broadcast()

	return nil
}

// We have to do this to avoid a circular dependency on db2sdk (cryptokeys -> db2sdk -> tailnet -> cryptokeys)
func toSDKKeys(keys []database.CryptoKey) []codersdk.CryptoKey {
	into := make([]codersdk.CryptoKey, 0, len(keys))
	for _, key := range keys {
		into = append(into, toSDK(key))
	}
	return into
}

func toSDK(key database.CryptoKey) codersdk.CryptoKey {
	return codersdk.CryptoKey{
		Feature:   codersdk.CryptoKeyFeature(key.Feature),
		Sequence:  key.Sequence,
		StartsAt:  key.StartsAt,
		DeletesAt: key.DeletesAt.Time,
		Secret:    key.Secret.String,
	}
}
