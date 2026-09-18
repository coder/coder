package testutil_test

import (
	"runtime"
	"testing"
	"weak"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/testutil"
)

func TestCleanup(t *testing.T) {
	t.Parallel()

	type payload struct {
		buf []byte
	}

	var (
		ran bool
		ptr weak.Pointer[payload]
	)
	// The parent's cleanup runs after its parallel subtests have finished,
	// while the finished subtest is still reachable from the parent, just as
	// every top-level test stays reachable from the test binary's root.
	t.Cleanup(func() {
		assert.True(t, ran, "cleanup did not run")
		runtime.GC() //nolint:revive // Intentional GC to check the closure is collectible.
		runtime.GC() //nolint:revive // Intentional GC to check the closure is collectible.
		assert.Nil(t, ptr.Value(), "cleanup closure kept its captured value alive")
	})
	t.Run("register", func(t *testing.T) {
		t.Parallel()
		p := &payload{buf: make([]byte, 1<<20)}
		ptr = weak.Make(p)
		testutil.Cleanup(t, func() {
			ran = len(p.buf) > 0
		})
	})
}
