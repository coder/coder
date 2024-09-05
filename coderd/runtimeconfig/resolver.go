package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/syncmap"
)

type NoopResolver struct{}

func NewNoopResolver() *NoopResolver {
	return &NoopResolver{}
}

func (NoopResolver) GetRuntimeSetting(context.Context, string) (string, error) {
	return "", EntryNotFound
}

func (NoopResolver) UpsertRuntimeSetting(context.Context, string, string) error {
	return EntryNotFound
}

func (NoopResolver) DeleteRuntimeSetting(context.Context, string) error {
	return EntryNotFound
}

// StoreResolver uses the database as the underlying store for runtime settings.
type StoreResolver struct {
	db Store
}

func NewStoreResolver(db Store) *StoreResolver {
	return &StoreResolver{db: db}
}

func (m StoreResolver) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	val, err := m.db.GetRuntimeConfig(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", xerrors.Errorf("%q: %w", key, EntryNotFound)
		}
		return "", xerrors.Errorf("fetch %q: %w", key, err)
	}

	return val, nil
}

func (m StoreResolver) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	err := m.db.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{Key: key, Value: val})
	if err != nil {
		return xerrors.Errorf("update %q: %w", key, err)
	}
	return nil
}

func (m StoreResolver) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return m.db.DeleteRuntimeConfig(ctx, key)
}

type NamespacedResolver struct {
	ns      string
	wrapped Resolver
}

func OrganizationResolver(orgID uuid.UUID, wrapped Resolver) NamespacedResolver {
	return NamespacedResolver{ns: orgID.String(), wrapped: wrapped}
}

func (m NamespacedResolver) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	return m.wrapped.GetRuntimeSetting(ctx, m.namespacedKey(key))
}

func (m NamespacedResolver) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	return m.wrapped.UpsertRuntimeSetting(ctx, m.namespacedKey(key), val)
}

func (m NamespacedResolver) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return m.wrapped.DeleteRuntimeSetting(ctx, m.namespacedKey(key))
}

func (m NamespacedResolver) namespacedKey(k string) string {
	return fmt.Sprintf("%s:%s", m.ns, k)
}

// MemoryCachedResolver is a super basic implementation of a cache for runtime
// settings. Essentially, it reuses the shared "cache" that all resolvers should
// use.
type MemoryCachedResolver struct {
	cache *syncmap.Map[string, cacheEntry]

	wrapped Resolver
}

func NewMemoryCachedResolver(cache *syncmap.Map[string, cacheEntry], wrapped Resolver) *MemoryCachedResolver {
	return &MemoryCachedResolver{
		cache:   cache,
		wrapped: wrapped,
	}
}

func (m *MemoryCachedResolver) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	cv, ok := m.cache.Load(key)
	if ok {
		return cv.value, nil
	}

	v, err := m.wrapped.GetRuntimeSetting(ctx, key)
	if err != nil {
		return "", err
	}
	m.cache.Store(key, cacheEntry{value: v, lastUpdated: time.Now()})
	return v, nil
}

func (m *MemoryCachedResolver) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	err := m.wrapped.UpsertRuntimeSetting(ctx, key, val)
	if err != nil {
		return err
	}
	m.cache.Store(key, cacheEntry{value: val, lastUpdated: time.Now()})
	return nil
}

func (m *MemoryCachedResolver) DeleteRuntimeSetting(ctx context.Context, key string) error {
	err := m.wrapped.DeleteRuntimeSetting(ctx, key)
	if err != nil {
		return err
	}
	m.cache.Delete(key)
	return nil
}
