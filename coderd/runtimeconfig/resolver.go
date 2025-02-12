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

// NoopResolver implements the Resolver interface
var _ Resolver = &NoopResolver{}

// NoopResolver is a useful test device.
type NoopResolver struct{}

func NewNoopResolver() *NoopResolver {
	return &NoopResolver{}
}

func (NoopResolver) GetRuntimeConfig(context.Context, string) (string, error) {
	return "", ErrEntryNotFound
}

func (NoopResolver) UpsertRuntimeConfig(context.Context, string, string) error {
	return ErrEntryNotFound
}

func (NoopResolver) DeleteRuntimeConfig(context.Context, string) error {
	return ErrEntryNotFound
}

// StoreResolver implements the Resolver interface
var _ Resolver = &StoreResolver{}

// StoreResolver uses the database as the underlying store for runtime settings.
type StoreResolver struct {
	db Store
}

func NewStoreResolver(db Store) *StoreResolver {
	return &StoreResolver{db: db}
}

func (m StoreResolver) GetRuntimeConfig(ctx context.Context, key string) (string, error) {
	val, err := m.db.GetRuntimeConfig(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", xerrors.Errorf("%q: %w", key, ErrEntryNotFound)
		}
		return "", xerrors.Errorf("fetch %q: %w", key, err)
	}

	return val, nil
}

func (m StoreResolver) UpsertRuntimeConfig(ctx context.Context, key, val string) error {
	err := m.db.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{Key: key, Value: val})
	if err != nil {
		return xerrors.Errorf("update %q: %w", key, err)
	}
	return nil
}

func (m StoreResolver) DeleteRuntimeConfig(ctx context.Context, key string) error {
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

func (m NamespacedResolver) GetRuntimeConfig(ctx context.Context, key string) (string, error) {
	return m.wrapped.GetRuntimeConfig(ctx, m.namespacedKey(key))
}

func (m NamespacedResolver) UpsertRuntimeConfig(ctx context.Context, key, val string) error {
	return m.wrapped.UpsertRuntimeConfig(ctx, m.namespacedKey(key), val)
}

func (m NamespacedResolver) DeleteRuntimeConfig(ctx context.Context, key string) error {
	return m.wrapped.DeleteRuntimeConfig(ctx, m.namespacedKey(key))
}

func (m NamespacedResolver) namespacedKey(k string) string {
	return fmt.Sprintf("%s:%s", m.ns, k)
}
