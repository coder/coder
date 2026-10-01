//go:build !unix

package agentbox

import "github.com/tetratelabs/wazero/experimental"

// newMemoryAllocator returns nil, which keeps wazero's default Go slice
// backed linear memory.
func newMemoryAllocator(uint64) experimental.MemoryAllocator {
	return nil
}
