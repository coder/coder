//go:build unix

package agentbox

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMmapMemory(t *testing.T) {
	t.Parallel()
	const page = wasmPageSize
	allocator := newMemoryAllocator(4 * page)

	t.Run("GrowsInPlace", func(t *testing.T) {
		t.Parallel()
		mem := allocator.Allocate(page, 16*page)
		buf := mem.Reallocate(page)
		require.Len(t, buf, page)
		assert.Equal(t, page, cap(buf))
		buf[0], buf[page-1] = 1, 2

		grown := mem.Reallocate(3 * page)
		require.Len(t, grown, 3*page)
		assert.Equal(t, unsafe.SliceData(buf), unsafe.SliceData(grown), "growth keeps the address")
		assert.Equal(t, byte(1), grown[0])
		assert.Equal(t, byte(2), grown[page-1])
		assert.Zero(t, grown[3*page-1], "new pages are zeroed")
		grown[3*page-1] = 3

		assert.Nil(t, mem.Reallocate(5*page), "growth past the engine limit fails")
		assert.Len(t, mem.Reallocate(4*page), 4*page)
		mem.Free()
		mem.Free()
	})

	t.Run("ZeroSize", func(t *testing.T) {
		t.Parallel()
		mem := allocator.Allocate(0, 0)
		buf := mem.Reallocate(0)
		require.NotNil(t, buf)
		assert.Empty(t, buf)
		assert.Nil(t, mem.Reallocate(page))
		mem.Free()
	})
}
