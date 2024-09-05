package runtimeconfig

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/util/syncmap"
)

// StoreManager is the shared singleton that produces resolvers for runtime configuration.
type StoreManager struct{}

func NewStoreManager() Manager {
	return &StoreManager{}
}

func (*StoreManager) DeploymentResolver(db Store) Resolver {
	return NewStoreResolver(db)
}

func (*StoreManager) OrganizationResolver(db Store, orgID uuid.UUID) Resolver {
	return OrganizationResolver(orgID, NewStoreResolver(db))
}

type cacheEntry struct {
	value       string
	lastUpdated time.Time
}

// MemoryCacheManager is an example of how a caching layer can be added to the
// resolver from the manager.
// TODO: Delete MemoryCacheManager and implement it properly in 'StoreManager'.
type MemoryCacheManager struct {
	cache *syncmap.Map[string, cacheEntry]
}

func NewMemoryCacheManager() *MemoryCacheManager {
	return &MemoryCacheManager{
		cache: syncmap.New[string, cacheEntry](),
	}
}

func (m *MemoryCacheManager) DeploymentResolver(db Store) Resolver {
	return NewMemoryCachedResolver(m.cache, NewStoreResolver(db))
}

func (m *MemoryCacheManager) OrganizationResolver(db Store, orgID uuid.UUID) Resolver {
	return OrganizationResolver(orgID, NewMemoryCachedResolver(m.cache, NewStoreResolver(db)))
}
