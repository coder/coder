package coderd

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
)

// RuntimeConfigStore TODO
type RuntimeConfigStore struct {
	resolver *runtimeconfig.StoreResolver
	mutator  *runtimeconfig.StoreMutator
}

func NewRuntimeConfigStore(store database.Store) *RuntimeConfigStore {
	return &RuntimeConfigStore{
		resolver: runtimeconfig.NewStoreResolver(store),
		mutator:  runtimeconfig.NewStoreMutator(store),
	}
}

func (r RuntimeConfigStore) GetRuntimeSetting(ctx context.Context, name string) (string, error) {
	return r.resolver.GetRuntimeSetting(ctx, name)
}

func (r RuntimeConfigStore) UpsertRuntimeSetting(ctx context.Context, name, val string) error {
	return r.mutator.UpsertRuntimeSetting(ctx, name, val)
}

func (r RuntimeConfigStore) DeleteRuntimeSetting(ctx context.Context, name string) error {
	return r.mutator.DeleteRuntimeSetting(ctx, name)
}
