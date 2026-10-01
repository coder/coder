//go:build unix

package agentbox

import (
	"math"

	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/sys/unix"
)

// newMemoryAllocator backs guest linear memory with anonymous mappings.
// limit caps the address space reserved per memory.
func newMemoryAllocator(limit uint64) experimental.MemoryAllocator {
	return experimental.MemoryAllocatorFunc(func(_, maxBytes uint64) experimental.LinearMemory {
		return &mmapMemory{reserve: min(maxBytes, limit)}
	})
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
	if size > m.reserve {
		return nil
	}
	if m.mapping == nil && m.heap == nil && m.reserve > 0 && m.reserve <= math.MaxInt {
		mapping, err := unix.Mmap(-1, 0, int(m.reserve), unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err == nil {
			m.mapping = mapping
		}
	}
	if m.mapping == nil {
		if grow := int(size) - len(m.heap); grow > 0 { //nolint:gosec // size <= reserve <= math.MaxInt or the heap path.
			m.heap = append(m.heap, make([]byte, grow)...)
		}
		if m.heap == nil {
			m.heap = []byte{}
		}
		return m.heap[:size]
	}
	if size > m.committed {
		// Wasm pages are 64 KiB, so both bounds are OS page aligned.
		if err := unix.Mprotect(m.mapping[m.committed:size], unix.PROT_READ|unix.PROT_WRITE); err != nil {
			return nil
		}
		m.committed = size
	}
	return m.mapping[:size:size]
}

func (m *mmapMemory) Free() {
	if m.mapping != nil {
		_ = unix.Munmap(m.mapping)
		m.mapping = nil
	}
	m.heap = nil
}
