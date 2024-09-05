package runtimeconfig

import (
	"sync"
	"time"
)

type memoryCache struct {
	stale time.Duration
	mu    sync.Mutex
}

func newMemoryCache(stale time.Duration) *memoryCache {
	return &memoryCache{stale: stale}
}

type MemoryCacheResolver struct {
}

func NewMemoryCacheResolver() *MemoryCacheResolver {
	return &MemoryCacheResolver{}
}
