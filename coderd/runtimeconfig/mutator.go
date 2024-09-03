package runtimeconfig

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type StoreMutator struct {
	store Store
}

func NewStoreMutator(store Store) *StoreMutator {
	if store == nil {
		panic("developer error: store is nil")
	}
	return &StoreMutator{store}
}

func (s StoreMutator) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	err := s.store.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{Key: key, Value: val})
	if err != nil {
		return xerrors.Errorf("update %q: %w", err)
	}
	return nil
}

func (s StoreMutator) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return s.store.DeleteRuntimeConfig(ctx, key)
}

type OrgMutator struct {
	inner Mutator
	orgID uuid.UUID
}

func NewOrgMutator(orgID uuid.UUID, inner Mutator) *OrgMutator {
	return &OrgMutator{inner: inner, orgID: orgID}
}

func (m OrgMutator) UpsertRuntimeSetting(ctx context.Context, key, val string) error {
	return m.inner.UpsertRuntimeSetting(ctx, orgKey(m.orgID, key), val)
}

func (m OrgMutator) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return m.inner.DeleteRuntimeSetting(ctx, key)
}

type NoopMutator struct{}

func NewNoopMutator() *NoopMutator {
	return &NoopMutator{}
}

func (n NoopMutator) UpsertRuntimeSetting(context.Context, string, string) error {
	return nil
}

func (n NoopMutator) DeleteRuntimeSetting(context.Context, string) error {
	return nil
}
