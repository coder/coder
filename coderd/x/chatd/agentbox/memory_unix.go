//go:build unix

package agentbox

import (
	"math"
	"sync"

	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/sys/unix"
)

// newRunMemory returns an allocator that backs one run's guest linear
// memory with anonymous mappings, and a function that frees every memory
// it allocated. limit caps the address space reserved per memory. The
// free function must only be called once the guest has returned.
func newRunMemory(limit uint64) (experimental.MemoryAllocator, func()) {
	var (
		mu       sync.Mutex
		memories []*mmapMemory
	)
	allocator := experimental.MemoryAllocatorFunc(func(_, maxBytes uint64) experimental.LinearMemory {
		mem := &mmapMemory{reserve: min(maxBytes, limit)}
		mu.Lock()
		memories = append(memories, mem)
		mu.Unlock()
		return mem
	})
	return allocator, func() {
		mu.Lock()
		defer mu.Unlock()
		for _, mem := range memories {
			mem.Free()
		}
		memories = nil
	}
}

// mmapMemory reserves its maximum size as an inaccessible mapping on
// first use and makes pages readable and writable as the guest grows, so
// growth never copies and the address stays fixed. Free unmaps it, which
// returns the pages to the OS at once. If the reservation fails it falls
// back to a Go slice that grows by copying.
type mmapMemory struct {
	reserve   uint64
	mapping   []byte
	committed uint64
	heap      []byte
}

func (m *mmapMemory) Reallocate(size uint64) []byte {
	if size > m.reserve || size > math.MaxInt {
		return nil
	}
	if m.mapping == nil && m.heap == nil && m.reserve > 0 && m.reserve <= math.MaxInt {
		mapping, err := unix.Mmap(-1, 0, int(m.reserve), unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err == nil {
			m.mapping = mapping
		}
	}
	if m.mapping != nil && size > m.committed {
		// Wasm pages are 64 KiB, so both bounds are OS page aligned.
		err := unix.Mprotect(m.mapping[m.committed:size], unix.PROT_READ|unix.PROT_WRITE)
		switch {
		case err == nil:
			m.committed = size
		case m.committed == 0:
			// wazero panics if the initial size cannot be provided.
			_ = unix.Munmap(m.mapping)
			m.mapping = nil
		default:
			return nil
		}
	}
	if m.mapping != nil {
		return m.mapping[:size:size]
	}
	if grow := int(size) - len(m.heap); grow > 0 { //nolint:gosec // size <= math.MaxInt is checked above.
		m.heap = append(m.heap, make([]byte, grow)...)
	}
	if m.heap == nil {
		m.heap = []byte{}
	}
	return m.heap[:size]
}

func (m *mmapMemory) Free() {
	if m.mapping != nil {
		_ = unix.Munmap(m.mapping)
		m.mapping = nil
	}
	m.heap = nil
}
