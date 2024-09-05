package runtimeconfig

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/util/syncmap"
)

type StoreManager struct {
}

func NewStoreManager() *StoreManager {
	return &StoreManager{}
}

func (m *StoreManager) DeploymentResolver(db Store) Resolver {
	return NewStoreResolver(db)
}

func (m *StoreManager) OrganizationResolver(db Store, orgID uuid.UUID) Resolver {
	return OrganizationResolver(orgID, NewStoreResolver(db))
}

type cacheEntry struct {
	value       string
	lastUpdated time.Time
}

type MemoryCacheManager struct {
	cache   *syncmap.Map[string, cacheEntry]
	wrapped Manager
}

func NewMemoryCacheManager(wrapped Manager) *MemoryCacheManager {
	return &MemoryCacheManager{
		cache:   syncmap.New[string, cacheEntry](),
		wrapped: wrapped,
	}
}

func (m *MemoryCacheManager) DeploymentResolver(db Store) Resolver {
	return NewMemoryCachedResolver(m.cache, m.wrapped.DeploymentResolver(db))
}

func (m *MemoryCacheManager) OrganizationResolver(db Store, orgID uuid.UUID) Resolver {
	return OrganizationResolver(orgID, NewMemoryCachedResolver(m.cache, m.wrapped.DeploymentResolver(db)))
}
