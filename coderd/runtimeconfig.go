package coderd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

// RuntimeConfigStore TODO
// Created here to avoid dependency cycle with database in runtimeconfig package.
type RuntimeConfigStore struct {
	store    *runtimeConfigStore
	resolver *runtimeconfig.StoreResolver
	mutator  *runtimeconfig.StoreMutator
}

func NewRuntimeConfigStore(store database.Store) *RuntimeConfigStore {
	runtimeStore := &runtimeConfigStore{store}
	return &RuntimeConfigStore{
		store:    runtimeStore,
		resolver: runtimeconfig.NewStoreResolver(runtimeStore),
		mutator:  runtimeconfig.NewStoreMutator(runtimeStore),
	}
}

func (r RuntimeConfigStore) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	return r.store.GetRuntimeSetting(ctx, key)
}

func (r RuntimeConfigStore) UpsertRuntimeSetting(ctx context.Context, key, value string) error {
	return r.store.UpsertRuntimeSetting(ctx, key, value)
}

func (r RuntimeConfigStore) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return r.store.DeleteRuntimeSetting(ctx, key)
}

func (r RuntimeConfigStore) ResolveByKey(ctx context.Context, key string) (string, error) {
	return r.resolver.ResolveByKey(ctx, key)
}

func (r RuntimeConfigStore) MutateByKey(ctx context.Context, key, val string) error {
	return r.mutator.MutateByKey(ctx, key, val)
}

type runtimeConfigStore struct {
	store database.Store
}

func (r runtimeConfigStore) GetRuntimeSetting(ctx context.Context, key string) (string, error) {
	return r.store.GetRuntimeConfig(ctx, key)
}

func (r runtimeConfigStore) UpsertRuntimeSetting(ctx context.Context, key, value string) error {
	return r.store.UpsertRuntimeConfig(ctx, database.UpsertRuntimeConfigParams{
		Key:   key,
		Value: value,
	})
}

func (r runtimeConfigStore) DeleteRuntimeSetting(ctx context.Context, key string) error {
	return r.store.DeleteRuntimeConfig(ctx, key)
}
