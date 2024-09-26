package cryptokeys

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
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
// it is not deleted or has breached its deletion date.
func (d *DBCache) Version(ctx context.Context, sequence int32) (database.CryptoKey, error) {
	now := d.clock.Now().UTC()
	d.cacheMu.RLock()
	key, ok := d.cache[sequence]
	d.cacheMu.RUnlock()
	if ok {
		if key.IsInvalid(now) {
			return database.CryptoKey{}, ErrKeyNotFound
		}
		return key, nil
	}

	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	key, ok = d.cache[sequence]
	if ok {
		return key, nil
	}

	key, err := d.db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
		Feature:  d.feature,
		Sequence: sequence,
	})
	if xerrors.Is(err, sql.ErrNoRows) {
		return database.CryptoKey{}, ErrKeyNotFound
	}
	if err != nil {
		return database.CryptoKey{}, err
	}

	if key.IsInvalid(now) {
		return database.CryptoKey{}, ErrKeyInvalid
	}

	if key.IsActive(now) && key.Sequence > d.latestKey.Sequence {
		d.latestKey = key
	}

	d.cache[sequence] = key

	return key, nil
}

func (d *DBCache) Latest(ctx context.Context) (database.CryptoKey, error) {
	d.cacheMu.RLock()
	latest := d.latestKey
	d.cacheMu.RUnlock()

	now := d.clock.Now().UTC()
	if latest.IsActive(now) {
		return latest, nil
	}

	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	if latest.IsActive(now) {
		return latest, nil
	}

	cache, latest, err := d.newCache(ctx)
	if err != nil {
		return database.CryptoKey{}, xerrors.Errorf("new cache: %w", err)
	}

	if len(cache) == 0 {
		return database.CryptoKey{}, ErrKeyNotFound
	}

	if !latest.IsActive(now) {
		return database.CryptoKey{}, ErrKeyInvalid
	}

	d.cache, d.latestKey = cache, latest

	return d.latestKey, nil
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

func (d *DBCache) newCache(ctx context.Context) (map[int32]database.CryptoKey, database.CryptoKey, error) {
	now := d.clock.Now().UTC()
	keys, err := d.db.GetCryptoKeysByFeature(ctx, d.feature)
	if err != nil {
		return nil, database.CryptoKey{}, xerrors.Errorf("get crypto keys by feature: %w", err)
	}
	cache := toMap(keys)
	var latest database.CryptoKey
	// Keys are returned in order from highest sequence to lowest.
	for _, key := range keys {
		if !key.IsActive(now) {
			continue
		}
		latest = key
		break
	}

	return cache, latest, nil
}

func toMap(keys []database.CryptoKey) map[int32]database.CryptoKey {
	m := make(map[int32]database.CryptoKey)
	for _, key := range keys {
		m[key.Sequence] = key
	}
	return m
}
