package cryptokeys

import (
	"context"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// never represents the maximum value for a time.Duration.
const never = 1<<63 - 1

// DBCache implements Keycache for callers with access to the database.
type DBCache struct {
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
}

type DBCacheOption func(*DBCache)

func WithDBCacheClock(clock quartz.Clock) DBCacheOption {
	return func(d *DBCache) {
		d.clock = clock
	}
}

// NewDBCache creates a new DBCache. Close should be called to
// release resources associated with its internal timer.
func NewDBCache(logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*DBCache)) *DBCache {
	d := &DBCache{
		db:      db,
		feature: feature,
		clock:   quartz.NewReal(),
		logger:  logger,
	}

	for _, opt := range opts {
		opt(d)
	}

	d.timer = d.clock.AfterFunc(never, d.clear)

	return d
}

// Verifying returns the CryptoKey with the given sequence number, provided that
// it is neither deleted nor has breached its deletion date. It should only be
// used for verifying or decrypting payloads. To sign/encrypt call Signing.
func (d *DBCache) Verifying(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	now := d.clock.Now()
	d.keysMu.RLock()
	key, ok := d.keys[sequence]
	d.keysMu.RUnlock()
	if ok {
		return checkKey(key, now)
	}

	d.keysMu.Lock()
	defer d.keysMu.Unlock()

	key, ok = d.keys[sequence]
	if ok {
		return checkKey(key, now)
	}

	err := d.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	key, ok = d.keys[sequence]
	if !ok {
		return codersdk.CryptoKey{}, ErrKeyNotFound
	}

	return checkKey(key, now)
}

// Signing returns the latest valid key for signing. A valid key is one that is
// both past its start time and before its deletion time.
func (d *DBCache) Signing(ctx context.Context) (codersdk.CryptoKey, error) {
	d.keysMu.RLock()
	latest := d.latestKey
	d.keysMu.RUnlock()

	now := d.clock.Now()
	if latest.CanSign(now) {
		return db2sdk.CryptoKey(latest), nil
	}

	d.keysMu.Lock()
	defer d.keysMu.Unlock()

	if d.latestKey.CanSign(now) {
		return db2sdk.CryptoKey(d.latestKey), nil
	}

	// Refetch all keys for this feature so we can find the latest valid key.
	err := d.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}

	return db2sdk.CryptoKey(d.latestKey), nil
}

// clear invalidates the cache. This forces the subsequent call to fetch fresh keys.
func (d *DBCache) clear() {
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
func (d *DBCache) fetch(ctx context.Context) error {
	keys, err := d.db.GetCryptoKeysByFeature(ctx, d.feature)
	if err != nil {
		return xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	now := d.clock.Now()
	d.timer.Stop()
	d.timer = d.newTimer()
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

func checkKey(key database.CryptoKey, now time.Time) (codersdk.CryptoKey, error) {
	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	return db2sdk.CryptoKey(key), nil
}

func (d *DBCache) newTimer() *quartz.Timer {
	return d.clock.AfterFunc(time.Minute*10, d.clear)
}

func (d *DBCache) Close() {
	d.keysMu.Lock()
	defer d.keysMu.Unlock()
	d.timer.Stop()
}
