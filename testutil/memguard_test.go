package testutil_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

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
