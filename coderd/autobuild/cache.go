package autobuild

import (
	"errors"
	"sync"
)

var errCacheLoaderNil = errors.New("loader is nil")

type cacheOf[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

func newCacheOf[K comparable, V any]() *cacheOf[K, V] {
	return &cacheOf[K, V]{
		m: make(map[K]V),
	}
}

func (c *cacheOf[K, V]) LoadOrStore(key K, loader func() (V, error)) (V, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, found := c.m[key]
	if !found {
		if loader == nil {
			return *new(V), errCacheLoaderNil
		}

		loaded, err := loader()
		if err != nil {
			return *new(V), err
		}

		c.m[key] = loaded
		return loaded, nil
	}

	return value, nil
}
