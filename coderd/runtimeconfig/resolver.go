package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type StoreResolver struct {
	store Store
}

func NewStoreResolver(store Store) *StoreResolver {
	return &StoreResolver{store}
}

func (s StoreResolver) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	if s.store == nil {
		panic("developer error: store must be set")
	}

	val, err := s.store.GetRuntimeConfig(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", xerrors.Errorf("%q: %w", key, EntryNotFound)
		}
		return "", xerrors.Errorf("fetch %q: %w", key, err)
	}

	return val, nil
}

type OrgResolver struct {
	inner Resolver
	orgID uuid.UUID
}

func NewOrgResolver(orgID uuid.UUID, inner Resolver) *OrgResolver {
	if inner == nil {
		panic("developer error: resolver is nil")
	}

	return &OrgResolver{inner: inner, orgID: orgID}
}

func (r OrgResolver) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	return r.inner.GetRuntimeSetting(ctx, orgKey(r.orgID, key))
}

// NoopResolver will always fail to resolve the given key.
// Useful in tests where you just want to look up the startup value of configs, and are not concerned with runtime config.
type NoopResolver struct{}

func NewNoopResolver() *NoopResolver {
	return &NoopResolver{}
}

func (n NoopResolver) GetRuntimeSetting(context.Context, string) (string, error) {
	return "", EntryNotFound
}
