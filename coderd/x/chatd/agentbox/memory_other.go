//go:build !unix

package agentbox

import "github.com/tetratelabs/wazero/experimental"

// newRunMemory returns a nil allocator, which keeps wazero's default Go
// slice backed linear memory, and a no-op free function.
func newRunMemory(uint64) (experimental.MemoryAllocator, func()) {
	return nil, func() {}
}
