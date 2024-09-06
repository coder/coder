package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// NoopResolver is a useful test device.
type NoopResolver struct{}

func NewNoopResolver() *NoopResolver {
	return &NoopResolver{}
}

func (NoopResolver) GetRuntimeSetting(context.Context, string) (string, error) {
	return "", ErrEntryNotFound
}

func (NoopResolver) UpsertRuntimeSetting(context.Context, string, string) error {
	return ErrEntryNotFound
}

func (NoopResolver) DeleteRuntimeSetting(context.Context, string) error {
	return ErrEntryNotFound
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
			return "", xerrors.Errorf("%q: %w", key, ErrEntryNotFound)
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

// NamespacedResolver prefixes all keys with a namespace.
// Then defers to the underlying resolver for the actual operations.
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
