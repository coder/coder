// Package objstoretest provides an in-memory Store implementation
// for use in tests.
package objstoretest

import (
	"bytes"
	"context"
	"io"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/coder/v2/coderd/objstore"
)

type memObject struct {
	data     []byte
	modified time.Time
}

// MemoryStore is an in-memory implementation of objstore.Store.
// It is safe for concurrent use.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string]memObject // full key = namespace/key
	closed  atomic.Bool
}

// NewMemory returns a Store backed entirely by memory. Useful for
// unit tests that need object storage but don't care about backend
// specifics.
func NewMemory() *MemoryStore {
	return &MemoryStore{
		objects: make(map[string]memObject),
	}
}

func (m *MemoryStore) Read(_ context.Context, namespace, key string) (io.ReadCloser, objstore.ObjectInfo, error) {
	if m.closed.Load() {
		return nil, objstore.ObjectInfo{}, objstore.ErrClosed
	}

	full := fullKey(namespace, key)

	m.mu.RLock()
	obj, ok := m.objects[full]
	m.mu.RUnlock()

	if !ok {
		return nil, objstore.ObjectInfo{}, objstore.ErrNotFound
	}

	info := objstore.ObjectInfo{
		Key:          key,
		Size:         int64(len(obj.data)),
		LastModified: obj.modified,
	}
	return io.NopCloser(bytes.NewReader(obj.data)), info, nil
}

func (m *MemoryStore) Write(_ context.Context, namespace, key string, data []byte) error {
	if m.closed.Load() {
		return objstore.ErrClosed
	}

	full := fullKey(namespace, key)

	// Copy to avoid retaining caller's slice.
	cp := make([]byte, len(data))
	copy(cp, data)

	m.mu.Lock()
	m.objects[full] = memObject{
		data:     cp,
		modified: time.Now(),
	}
	m.mu.Unlock()

	return nil
}

func (m *MemoryStore) List(_ context.Context, namespace, prefix string) iter.Seq2[objstore.ObjectInfo, error] {
	return func(yield func(objstore.ObjectInfo, error) bool) {
		if m.closed.Load() {
			yield(objstore.ObjectInfo{}, objstore.ErrClosed)
			return
		}

		fullPrefix := namespace + "/"
		if prefix != "" {
			fullPrefix += prefix
		}

		m.mu.RLock()
		defer m.mu.RUnlock()

		for k, obj := range m.objects {
			if !strings.HasPrefix(k, fullPrefix) {
				continue
			}

			relKey := k[len(namespace)+1:]
			info := objstore.ObjectInfo{
				Key:          relKey,
				Size:         int64(len(obj.data)),
				LastModified: obj.modified,
			}
			if !yield(info, nil) {
				return
			}
		}
	}
}

func (m *MemoryStore) Delete(_ context.Context, namespace, key string) error {
	if m.closed.Load() {
		return objstore.ErrClosed
	}

	full := fullKey(namespace, key)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.objects[full]; !ok {
		return objstore.ErrNotFound
	}
	delete(m.objects, full)
	return nil
}

func (m *MemoryStore) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	return nil
}

func fullKey(namespace, key string) string {
	return namespace + "/" + key
}

// Compile-time check.
var _ objstore.Store = (*MemoryStore)(nil)
