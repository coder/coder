package oidctest

import "sync"

// SyncMap is a type safe sync.Map
type SyncMap[K, V any] struct {
	m sync.Map
}

func NewSyncMap[K, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: sync.Map{},
	}
}

func (s *SyncMap[K, V]) Store(k K, v V) {
	s.m.Store(k, v)
}

func (s *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	v, ok := s.m.Load(key)
	if !ok {
		var empty V
		return empty, false
	}
	return v.(V), ok
}

func (m *SyncMap[K, V]) Delete(key K) {
	m.m.Delete(key)
}

func (m *SyncMap[K, V]) LoadAndDelete(key K) (actual V, loaded bool) {
	act, loaded := m.m.LoadAndDelete(key)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return act.(V), loaded
}

func (m *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	act, loaded := m.m.LoadOrStore(key, value)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return act.(V), loaded
}

func (m *SyncMap[K, V]) CompareAndSwap(key K, old V, new V) bool {
	return m.m.CompareAndSwap(key, old, new)
}

func (m *SyncMap[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

func (m *SyncMap[K, V]) Swap(key K, value V) (previous any, loaded bool) {
	previous, loaded = m.m.Swap(key, value)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return previous.(V), loaded
}

func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value interface{}) bool {
		return f(key.(K), value.(V))
	})
}
