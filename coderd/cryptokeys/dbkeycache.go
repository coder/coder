package cryptokeys

import (
	"context"
	"database/sql"
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
	cacheMu   sync.RWMutex
	cache     map[int32]database.CryptoKey
	latestKey database.CryptoKey
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
func NewDBCache(ctx context.Context, logger slog.Logger, db database.Store, feature database.CryptoKeyFeature, opts ...func(*DBCache)) (*DBCache, error) {
	d := &DBCache{
		db:      db,
		feature: feature,
		clock:   quartz.NewReal(),
		logger:  logger,
	}
	for _, opt := range opts {
		opt(d)
	}

	cache, latest, err := d.newCache(ctx)
	if err != nil {
		return nil, xerrors.Errorf("new cache: %w", err)
	}
	d.cache, d.latestKey = cache, latest

	go d.refresh(ctx)
	return d, nil
}

// Version returns the CryptoKey with the given sequence number, provided that
// it is neither deleted nor has breached its deletion date.
func (d *DBCache) Version(ctx context.Context, sequence int32) (codersdk.CryptoKey, error) {
	now := d.clock.Now().UTC()
	d.cacheMu.RLock()
	key, ok := d.cache[sequence]
	d.cacheMu.RUnlock()
	if ok {
		return checkKey(key, now)
	}

	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	key, ok = d.cache[sequence]
	if ok {
		return checkKey(key, now)
	}

	key, err := d.db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
		Feature:  d.feature,
		Sequence: sequence,
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		return codersdk.CryptoKey{}, ErrKeyNotFound
	}
	if err != nil {
		return codersdk.CryptoKey{}, err
	}

	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	// If this key is valid for signing then mark it as the latest key.
	if key.CanSign(now) && key.Sequence > d.latestKey.Sequence {
		d.latestKey = key
	}

	d.cache[sequence] = key

	return db2sdk.CryptoKey(key), nil
}

// Latest returns the latest valid key for signing. A valid key is one that is
// both past its start time and before its deletion time.
func (d *DBCache) Latest(ctx context.Context) (codersdk.CryptoKey, error) {
	d.cacheMu.RLock()
	latest := d.latestKey
	d.cacheMu.RUnlock()

	now := d.clock.Now().UTC()
	if latest.CanSign(now) {
		return checkKey(latest, now)
	}

	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	if latest.CanSign(now) {
		return checkKey(latest, now)
	}

	// Refetch all keys for this feature so we can find the latest valid key.
	cache, latest, err := d.newCache(ctx)
	if err != nil {
		return codersdk.CryptoKey{}, xerrors.Errorf("new cache: %w", err)
	}

	if len(cache) == 0 {
		return codersdk.CryptoKey{}, ErrKeyNotFound
	}

	if !latest.CanSign(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	d.cache, d.latestKey = cache, latest

	return checkKey(latest, now)
}

func (d *DBCache) refresh(ctx context.Context) {
	d.clock.TickerFunc(ctx, time.Minute*10, func() error {
		cache, latest, err := d.newCache(ctx)
		if err != nil {
			d.logger.Error(ctx, "failed to refresh cache", slog.Error(err))
			return nil
		}
		d.cacheMu.Lock()
		defer d.cacheMu.Unlock()

		d.cache, d.latestKey = cache, latest
		return nil
	})
}

// newCache fetches all keys for the given feature and determines the latest key.
func (d *DBCache) newCache(ctx context.Context) (map[int32]database.CryptoKey, database.CryptoKey, error) {
	now := d.clock.Now().UTC()
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

	return cache, latest, nil
}

func checkKey(key database.CryptoKey, now time.Time) (codersdk.CryptoKey, error) {
	if !key.CanVerify(now) {
		return codersdk.CryptoKey{}, ErrKeyInvalid
	}

	return db2sdk.CryptoKey(key), nil
}
