package syncmap

import (
	"sync"
)

// Map is a type safe sync.Map
type Map[K, V any] struct {
	m sync.Map
}

func New[K, V any]() *Map[K, V] {
	return &Map[K, V]{
		m: sync.Map{},
	}
}

func (m *Map[K, V]) Store(k K, v V) {
	m.m.Store(k, v)
}

//nolint:forcetypeassert
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.m.Load(key)
	if !ok {
		var empty V
		return empty, false
	}
	return v.(V), ok
}

func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}

//nolint:forcetypeassert
func (m *Map[K, V]) LoadAndDelete(key K) (actual V, loaded bool) {
	act, loaded := m.m.LoadAndDelete(key)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return act.(V), loaded
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value. The loaded result
// is true if the value was loaded, false if stored. As with sync.Map,
// actual is usable in both cases.
//
//nolint:forcetypeassert
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	act, loaded := m.m.LoadOrStore(key, value)
	return act.(V), loaded
}

func (m *Map[K, V]) CompareAndSwap(key K, old V, newVal V) bool {
	return m.m.CompareAndSwap(key, old, newVal)
}

func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

// Swap stores the given value for the key and returns the previous
// value if there was one. As with sync.Map, previous is the zero V when
// the key was absent.
//
//nolint:forcetypeassert
func (m *Map[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	prev, loaded := m.m.Swap(key, value)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return prev.(V), loaded
}

//nolint:forcetypeassert
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value interface{}) bool {
		return f(key.(K), value.(V))
	})
}
