package testutil_test

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/pprof/profile"
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

func TestStartMemoryGuardFromEnv(t *testing.T) {
	t.Run("Unset", func(t *testing.T) {
		t.Setenv(testutil.MemoryLimitEnv, "")
		stop := testutil.StartMemoryGuardFromEnv()
		stop()
	})

	t.Run("InvalidValue", func(t *testing.T) {
		t.Setenv(testutil.MemoryLimitEnv, "not-a-number")
		require.Panics(t, func() {
			testutil.StartMemoryGuardFromEnv()
		})
	})
}

// memoryGuardFromEnvSubprocessEnv marks the re-executed test binary that
// runs the env-configured guard until it panics.
const memoryGuardFromEnvSubprocessEnv = "CODER_TEST_MEMORY_GUARD_FROM_ENV_SUBPROCESS"

func TestStartMemoryGuardFromEnvTrips(t *testing.T) {
	if os.Getenv(memoryGuardFromEnvSubprocessEnv) == "1" {
		testutil.StartMemoryGuardFromEnv()
		<-time.After(testutil.WaitLong)
		t.Fatal("memory guard did not trip")
	}
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gosec // Test-controlled arguments.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(),
		memoryGuardFromEnvSubprocessEnv+"=1",
		testutil.MemoryLimitEnv+"=0",
		"RUNNER_TEMP="+t.TempDir())
	out, err := cmd.CombinedOutput()
	t.Logf("subprocess output:\n%s", out)

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "subprocess did not fail")
	require.Equal(t, 2, exitErr.ExitCode(), "subprocess did not exit from a panic")
	require.Contains(t, string(out), "panic: memory guard: test process uses")
	require.Contains(t, string(out), "over the 0 MiB limit")
}

// memoryGuardSubprocessEnv marks the re-executed test binary that runs
// the guard until it trips.
const memoryGuardSubprocessEnv = "CODER_TEST_MEMORY_GUARD_SUBPROCESS"

func TestStartMemoryGuardTrips(t *testing.T) {
	if os.Getenv(memoryGuardSubprocessEnv) == "1" {
		testutil.StartMemoryGuard(1)
		<-time.After(testutil.WaitLong)
		t.Fatal("memory guard did not trip")
	}
	t.Parallel()

	dir := t.TempDir()
	ctx := testutil.Context(t, testutil.WaitLong)
	//nolint:gosec // Test-controlled arguments.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), memoryGuardSubprocessEnv+"=1", "RUNNER_TEMP="+dir)
	out, err := cmd.CombinedOutput()
	t.Logf("subprocess output:\n%s", out)

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "subprocess did not fail")
	require.Equal(t, 2, exitErr.ExitCode(), "subprocess did not exit from a panic")
	require.Contains(t, string(out), "panic: memory guard: test process uses")
	require.Contains(t, string(out), "over the 0 MiB limit")
	require.Contains(t, string(out), "heap profile after collection:")
	// The test goroutine is not the one that panics, so its presence
	// shows the dump covers every goroutine.
	require.Contains(t, string(out), t.Name()+"(")

	profiles, err := filepath.Glob(filepath.Join(dir, "heap-*.pprof"))
	require.NoError(t, err)
	require.Len(t, profiles, 2, "expected an immediate and a post-collection profile")
	for _, path := range profiles {
		f, err := os.Open(path)
		require.NoError(t, err)
		prof, err := profile.Parse(f)
		_ = f.Close()
		require.NoError(t, err, "profile %s is not readable", path)
		require.NotEmpty(t, prof.Sample, "profile %s has no samples", path)
	}
}

func TestWriteHeapProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path, err := testutil.WriteHeapProfile(dir, "")
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

	immediate, collected, err := testutil.WriteHeapProfiles(t.TempDir(), testutil.WaitLong)
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
