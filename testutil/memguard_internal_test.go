package testutil

import (
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"
)

func TestStartMemoryGuard(t *testing.T) {
	t.Parallel()

	t.Run("Exceeded", func(t *testing.T) {
		t.Parallel()

		exceeded := make(chan uint64, 1)
		stop := watchMemory(1, func(total uint64) {
			exceeded <- total
		})
		defer stop()

		select {
		case total := <-exceeded:
			require.NotZero(t, total)
		case <-time.After(WaitShort):
			t.Fatal("memory guard did not trip")
		}
	})

	t.Run("Stops", func(t *testing.T) {
		t.Parallel()

		stop := watchMemory(math.MaxUint64, func(uint64) {
			t.Error("memory guard tripped below the limit")
		})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			stop()
		}()
		select {
		case <-stopped:
		case <-time.After(WaitShort):
			t.Fatal("memory guard did not stop")
		}
	})
}

func TestWriteHeapProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path, err := writeHeapProfile(dir, "")
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Size())
}

func TestWriteHeapProfiles(t *testing.T) {
	t.Parallel()

	// Two collections publish everything allocated so far. The burst
	// below then sits in an unpublished profile cycle, as it would when
	// the guard trips, until the forced collection in WriteHeapProfiles
	// publishes it. The allocation is large enough that the profiler
	// cannot skip it at the default sampling rate.
	runtime.GC() //nolint:revive // Two collections publish everything allocated so far.
	runtime.GC() //nolint:revive // See above.
	burst := allocateBurst()

	immediate, collected, err := writeHeapProfiles(t.TempDir(), WaitLong)
	require.NoError(t, err)
	require.FileExists(t, immediate)
	require.NotEmpty(t, collected, "collection did not finish")
	require.True(t, allocatedIn(t, collected, "allocateBurst"),
		"profile after collection misses the burst")
	runtime.KeepAlive(burst)
}

//go:noinline
func allocateBurst() []byte {
	return make([]byte, 32<<20)
}

// allocatedIn reports whether the heap profile at path attributes
// allocated space to a function with the given name.
func allocatedIn(t *testing.T, path, function string) bool {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	prof, err := profile.Parse(f)
	require.NoError(t, err)

	allocSpace := -1
	for i, st := range prof.SampleType {
		if st.Type == "alloc_space" {
			allocSpace = i
		}
	}
	require.NotEqual(t, -1, allocSpace, "profile has no alloc_space samples")

	for _, s := range prof.Sample {
		if s.Value[allocSpace] == 0 {
			continue
		}
		for _, loc := range s.Location {
			for _, line := range loc.Line {
				if strings.HasSuffix(line.Function.Name, "."+function) {
					return true
				}
			}
		}
	}
	return false
}
