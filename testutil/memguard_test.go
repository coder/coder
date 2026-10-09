package testutil_test

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestStartMemoryGuard(t *testing.T) {
	t.Parallel()

	t.Run("Exceeded", func(t *testing.T) {
		t.Parallel()

		exceeded := make(chan uint64, 1)
		stop := testutil.WatchMemory(1, func(total uint64) {
			exceeded <- total
		})
		defer stop()

		select {
		case total := <-exceeded:
			require.NotZero(t, total)
		case <-time.After(testutil.WaitShort):
			t.Fatal("memory guard did not trip")
		}
	})

	t.Run("Stops", func(t *testing.T) {
		t.Parallel()

		stop := testutil.WatchMemory(math.MaxUint64, func(uint64) {
			t.Error("memory guard tripped below the limit")
		})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			stop()
		}()
		select {
		case <-stopped:
		case <-time.After(testutil.WaitShort):
			t.Fatal("memory guard did not stop")
		}
	})
}

func TestWriteHeapProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path, err := testutil.WriteHeapProfile(dir)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
}
