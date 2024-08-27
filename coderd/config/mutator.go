package config

import (
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

func (s *StoreMutator) MutateByKey(key, val string) error {
	err := s.store.UpsertRuntimeSetting(key, val)
	if err != nil {
		return xerrors.Errorf("update %q: %w", err)
	}
	return nil
}

type OrgMutator struct {
	inner Mutator
	orgID uuid.UUID
}

func NewOrgMutator(inner Mutator, orgID uuid.UUID) *OrgMutator {
	return &OrgMutator{inner: inner, orgID: orgID}
}

func (m OrgMutator) MutateByKey(key, val string) error {
	return m.inner.MutateByKey(orgKey(m.orgID, key), val)
}
