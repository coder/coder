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

// cast converts a value returned by the underlying sync.Map to T. The
// map returns a nil `any` for a missing key, and for a present key whose
// interface-typed value is nil. Neither can be type-asserted, so both
// become the zero T, which is nil for interface types.
func cast[T any](v any) T {
	if v == nil {
		var empty T
		return empty
	}
	//nolint:forcetypeassert // Only K and V values ever enter the map.
	return v.(T)
}

func (m *Map[K, V]) Store(k K, v V) {
	m.m.Store(k, v)
}

func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.m.Load(key)
	return cast[V](v), ok
}

func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}

func (m *Map[K, V]) LoadAndDelete(key K) (actual V, loaded bool) {
	act, loaded := m.m.LoadAndDelete(key)
	return cast[V](act), loaded
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value. The loaded result
// is true if the value was loaded, false if stored. As with sync.Map,
// actual is usable in both cases.
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	act, loaded := m.m.LoadOrStore(key, value)
	return cast[V](act), loaded
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
func (m *Map[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	prev, loaded := m.m.Swap(key, value)
	return cast[V](prev), loaded
}

func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value any) bool {
		return f(cast[K](key), cast[V](value))
	})
}
