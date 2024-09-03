package runtimeconfig

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
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

func (s *StoreMutator) MutateByKey(ctx context.Context, key, val string) error {
	err := s.store.UpsertRuntimeSetting(ctx, key, val)
	if err != nil {
		return xerrors.Errorf("update %q: %w", err)
	}
	return nil
}

type OrgMutator struct {
	inner Mutator
	orgID uuid.UUID
}

func NewOrgMutator(orgID uuid.UUID, inner Mutator) *OrgMutator {
	return &OrgMutator{inner: inner, orgID: orgID}
}

func (m OrgMutator) MutateByKey(ctx context.Context, key, val string) error {
	return m.inner.MutateByKey(ctx, orgKey(m.orgID, key), val)
}

type NoopMutator struct{}

func (n *NoopMutator) MutateByKey(ctx context.Context, key, val string) error {
	return nil
}

func NewNoopMutator() *NoopMutator {
	return &NoopMutator{}
}
