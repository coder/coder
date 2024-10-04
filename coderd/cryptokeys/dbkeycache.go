package cryptokeys

import (
	"context"
	"strconv"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

// never represents the maximum value for a time.Duration.
const never = 1<<63 - 1

// dbCache implements Keycache for callers with access to the database.
type dbCache struct {
	db      database.Store
	feature database.CryptoKeyFeature
	logger  slog.Logger
	clock   quartz.Clock

	// The following are initialized by NewDBCache.
	keysMu    sync.RWMutex
	keys      map[int32]database.CryptoKey
	latestKey database.CryptoKey
	timer     *quartz.Timer
	// invalidateAt is the time at which the keys cache should be invalidated.
	invalidateAt time.Time
	closed       bool
}

type DBCacheOption func(*dbCache)

func WithDBCacheClock(clock quartz.Clock) DBCacheOption {
	return func(d *dbCache) {
		d.clock = clock
	}
}

// NewSigningCache creates a new DBCache. Close should be called to
// release resources associated with its internal timer.
func NewSigningCache(logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*dbCache)) (SigningKeycache, error) {
	if !isSigningKeyFeature(feature) {
		return nil, ErrInvalidFeature
	}

	return newDBCache(logger, db, feature, opts...), nil
}

func NewEncryptionCache(logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*dbCache)) (EncryptionKeycache, error) {
	if !isEncryptionKeyFeature(feature) {
		return nil, ErrInvalidFeature
	}

	return newDBCache(logger, db, feature, opts...), nil
}

func newDBCache(logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*dbCache)) *dbCache {
	d := &dbCache{
		db:      db,
		feature: feature,
		clock:   quartz.NewReal(),
		logger:  logger,
	}

	for _, opt := range opts {
		opt(d)
	}

	// Initialize the timer. This will get properly initialized the first time we fetch.
	d.timer = d.clock.AfterFunc(never, d.clear)

	return d
}

func (d *dbCache) EncryptingKey(ctx context.Context) (id string, key interface{}, err error) {
	if !isEncryptionKeyFeature(d.feature) {
		return "", nil, ErrInvalidFeature
	}

	return d.latest(ctx)
}

func (d *dbCache) DecryptingKey(ctx context.Context, id string) (key interface{}, err error) {
	if !isEncryptionKeyFeature(d.feature) {
		return nil, ErrInvalidFeature
	}

	return d.sequence(ctx, id)
}

func (d *dbCache) SigningKey(ctx context.Context) (id string, key interface{}, err error) {
	if !isSigningKeyFeature(d.feature) {
		return "", nil, ErrInvalidFeature
	}

	return d.latest(ctx)
}

func (d *dbCache) VerifyingKey(ctx context.Context, id string) (key interface{}, err error) {
	if !isSigningKeyFeature(d.feature) {
		return nil, ErrInvalidFeature
	}

	return d.sequence(ctx, id)
}

// sequence returns the CryptoKey with the given sequence number, provided that
// it is neither deleted nor has breached its deletion date. It should only be
// used for verifying or decrypting payloads. To sign/encrypt call Signing.
func (d *dbCache) sequence(ctx context.Context, id string) (interface{}, error) {
	sequence, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return nil, xerrors.Errorf("expecting sequence number got %q: %w", id, err)
	}

	d.keysMu.RLock()
	if d.closed {
		d.keysMu.RUnlock()
		return nil, ErrClosed
	}

	now := d.clock.Now()
	key, ok := d.keys[int32(sequence)]
	d.keysMu.RUnlock()
	if ok {
		return checkKey(key, now)
	}

	d.keysMu.Lock()
	defer d.keysMu.Unlock()

	if d.closed {
		return nil, ErrClosed
	}

	key, ok = d.keys[int32(sequence)]
	if ok {
		return checkKey(key, now)
	}

	err = d.fetch(ctx)
	if err != nil {
		return nil, xerrors.Errorf("fetch: %w", err)
	}

	key, ok = d.keys[int32(sequence)]
	if !ok {
		return nil, ErrKeyNotFound
	}

	return checkKey(key, now)
}

// latest returns the latest valid key for signing. A valid key is one that is
// both past its start time and before its deletion time.
func (d *dbCache) latest(ctx context.Context) (string, interface{}, error) {
	d.keysMu.RLock()

	if d.closed {
		d.keysMu.RUnlock()
		return "", nil, ErrClosed
	}

	latest := d.latestKey
	d.keysMu.RUnlock()

	now := d.clock.Now()
	if latest.CanSign(now) {
		return idSecret(latest)
	}

	d.keysMu.Lock()
	defer d.keysMu.Unlock()

	if d.closed {
		return "", nil, ErrClosed
	}

	if d.latestKey.CanSign(now) {
		return idSecret(d.latestKey)
	}

	// Refetch all keys for this feature so we can find the latest valid key.
	err := d.fetch(ctx)
	if err != nil {
		return "", nil, xerrors.Errorf("fetch: %w", err)
	}

	return idSecret(d.latestKey)
}

// clear invalidates the cache. This forces the subsequent call to fetch fresh keys.
func (d *dbCache) clear() {
	now := d.clock.Now("DBCache", "clear")
	d.keysMu.Lock()
	defer d.keysMu.Unlock()
	// Check if we raced with a fetch. It's possible that the timer fired and we
	// lost the race to the mutex. We want to avoid invalidating
	// a cache that was just refetched.
	if now.Before(d.invalidateAt) {
		return
	}
	d.keys = nil
	d.latestKey = database.CryptoKey{}
}

// fetch fetches all keys for the given feature and determines the latest key.
// It must be called while holding the keysMu lock.
func (d *dbCache) fetch(ctx context.Context) error {
	keys, err := d.db.GetCryptoKeysByFeature(ctx, d.feature)
	if err != nil {
		return xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	now := d.clock.Now()
	_ = d.timer.Reset(time.Minute * 10)
	d.invalidateAt = now.Add(time.Minute * 10)

	cache := make(map[int32]database.CryptoKey)
	var latest database.CryptoKey
	for _, key := range keys {
		cache[key.Sequence] = key
		if key.CanSign(now) && key.Sequence > latest.Sequence {
			latest = key
		}
	}

	if len(cache) == 0 {
		return ErrKeyNotFound
	}

	if !latest.CanSign(now) {
		return ErrKeyInvalid
	}

	d.keys, d.latestKey = cache, latest
	return nil
}

func checkKey(key database.CryptoKey, now time.Time) (interface{}, error) {
	if !key.CanVerify(now) {
		return nil, ErrKeyInvalid
	}

	return key.DecodeString()
}

func (d *dbCache) Close() error {
	d.keysMu.Lock()
	defer d.keysMu.Unlock()

	if d.closed {
		return nil
	}

	d.timer.Stop()
	d.closed = true
	return nil
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
