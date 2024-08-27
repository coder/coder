package config

import (
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

func (s StoreResolver) ResolveByKey(key string) (string, error) {
	if s.store == nil {
		panic("developer error: store must be set")
	}

	val, err := s.store.GetRuntimeSetting(key)
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

func NewOrgResolver(inner Resolver, orgID uuid.UUID) *OrgResolver {
	if inner == nil {
		panic("developer error: resolver is nil")
	}

	return &OrgResolver{inner: inner, orgID: orgID}
}

func (r OrgResolver) ResolveByKey(key string) (string, error) {
	return r.inner.ResolveByKey(orgKey(r.orgID, key))
}
