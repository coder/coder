package syncmap

import "sync"

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

//nolint:forcetypeassert
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	act, loaded := m.m.LoadOrStore(key, value)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return act.(V), loaded
}

func (m *Map[K, V]) CompareAndSwap(key K, old V, newVal V) bool {
	return m.m.CompareAndSwap(key, old, newVal)
}

func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

//nolint:forcetypeassert
func (m *Map[K, V]) Swap(key K, value V) (previous any, loaded bool) {
	previous, loaded = m.m.Swap(key, value)
	if !loaded {
		var empty V
		return empty, loaded
	}
	return previous.(V), loaded
}

//nolint:forcetypeassert
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value interface{}) bool {
		return f(key.(K), value.(V))
	})
}
