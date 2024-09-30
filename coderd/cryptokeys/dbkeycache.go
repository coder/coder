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
	fetched   chan struct{}
}

type DBCacheOption func(*DBCache)

func WithDBCacheClock(clock quartz.Clock) DBCacheOption {
	return func(d *DBCache) {
		d.clock = clock
	}
}

// NewDBCache creates a new DBCache. It starts a background
// process that periodically refreshes the cache. The context should
// be canceled to stop the background process.
func NewDBCache(ctx context.Context, logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*DBCache)) *DBCache {
	d := &DBCache{
		db:      db,
		feature: feature,
		clock:   quartz.NewReal(),
		logger:  logger,
		fetched: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(d)
	}

	go d.clear(ctx)
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

	cache, latest, err := d.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}
	d.keys, d.latestKey = cache, latest

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
	cache, latest, err := d.fetch(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("fetch: %w", err)
	}
	d.keys, d.latestKey = cache, latest

	return db2sdk.CryptoKey(d.latestKey), nil
}

func (d *DBCache) clear(ctx context.Context) {
	for {
		fired := make(chan struct{})
		timer := d.clock.AfterFunc(time.Minute*10, func() {
			defer close(fired)

			// There's a small window where the timer fires as we're fetching
			// keys that could result in us immediately invalidating the cache that we just populated.
			d.keysMu.Lock()
			defer d.keysMu.Unlock()
			d.keys = nil
			d.latestKey = database.CryptoKey{}
		})

		select {
		case <-ctx.Done():
			return
		case <-d.fetched:
			timer.Stop()
		case <-fired:
		}
	}
}

// fetch fetches all keys for the given feature and determines the latest key.
func (d *DBCache) fetch(ctx context.Context) (map[int32]database.CryptoKey, database.CryptoKey, error) {
	now := d.clock.Now()
	keys, err := d.db.GetCryptoKeysByFeature(ctx, d.feature)
	if err != nil {
		return nil, database.CryptoKey{}, xerrors.Errorf("get crypto keys by feature: %w", err)
	}

	cache := make(map[int32]database.CryptoKey)
	var latest database.CryptoKey
	for _, key := range keys {
		cache[key.Sequence] = key
		if key.CanSign(now) && key.Sequence > latest.Sequence {
			latest = key
		}
	}

	if len(cache) == 0 {
		return nil, database.CryptoKey{}, ErrKeyNotFound
	}

	if !latest.CanSign(now) {
		return nil, database.CryptoKey{}, ErrKeyInvalid
	}

	select {
	case <-ctx.Done():
		return nil, database.CryptoKey{}, ctx.Err()
	case d.fetched <- struct{}{}:
	}

	return cache, latest, nil
}

func checkKey(key database.CryptoKey, now time.Time) (codersdk.CryptoKey, error) {
	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	return db2sdk.CryptoKey(key), nil
}
