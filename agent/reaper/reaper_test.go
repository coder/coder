//go:build linux

package reaper_test

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/go-reap"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/reaper"
	"github.com/coder/coder/v2/testutil"
)

// subprocessEnvKey is set when a test re-execs itself as an
// isolated subprocess. Tests that call ForkReap or send signals
// to their own process check this to decide whether to run real
// test logic or launch the subprocess and wait for it.
const subprocessEnvKey = "CODER_REAPER_TEST_SUBPROCESS"

// runSubprocess re-execs the current test binary in a new process
// running only the named test. This isolates ForkReap's
// syscall.ForkExec and any process-directed signals (e.g. SIGINT)
// from the parent test binary, making these tests safe to run in
// CI and alongside other tests.
//
// Returns true inside the subprocess (caller should proceed with
// the real test logic). Returns false in the parent after the
// subprocess exits successfully (caller should return).
func runSubprocess(t *testing.T) bool {
	t.Helper()

	if os.Getenv(subprocessEnvKey) == "1" {
		return true
	}

	ctx := testutil.Context(t, testutil.WaitMedium)

	//nolint:gosec // Test-controlled arguments.
	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^"+t.Name()+"$",
		"-test.v",
	)
	cmd.Env = append(os.Environ(), subprocessEnvKey+"=1")

	out, err := cmd.CombinedOutput()
	t.Logf("Subprocess output:\n%s", out)
	require.NoError(t, err, "subprocess failed")

	return false
}

// withDone returns options that stop the reaper goroutine when t
// completes and wait for it to fully exit, preventing
// overlapping reapers across sequential subtests.
func withDone(t *testing.T) []reaper.Option {
	t.Helper()
	stop := make(chan struct{})
	stopped := make(chan struct{})
	t.Cleanup(func() {
		close(stop)
		<-stopped
	})
	return []reaper.Option{
		reaper.WithReaperStop(stop),
		reaper.WithReaperStopped(stopped),
	}
}

// TestReap checks that the reaper successfully reaps exited
// processes and passes their PIDs through the shared channel.
func TestReap(t *testing.T) {
	t.Parallel()
	if !runSubprocess(t) {
		return
	}

	pids := make(reap.PidCh, 1)
	var reapLock sync.RWMutex
	opts := append([]reaper.Option{
		reaper.WithPIDCallback(pids),
		reaper.WithExecArgs("/bin/sh", "-c", "exit 0"),
		reaper.WithReapLock(&reapLock),
	}, withDone(t)...)
	reapLock.RLock()
	exitCode, err := reaper.ForkReap(opts...)
	reapLock.RUnlock()
	require.NoError(t, err)
	require.Equal(t, 0, exitCode)

	cmd := exec.Command("tail", "-f", "/dev/null")
	err = cmd.Start()
	require.NoError(t, err)

	cmd2 := exec.Command("tail", "-f", "/dev/null")
	err = cmd2.Start()
	require.NoError(t, err)

	err = cmd.Process.Kill()
	require.NoError(t, err)

	err = cmd2.Process.Kill()
	require.NoError(t, err)

	expectedPIDs := []int{cmd.Process.Pid, cmd2.Process.Pid}

	for range len(expectedPIDs) {
		select {
		case <-time.After(testutil.WaitShort):
			t.Fatalf("Timed out waiting for process")
		case pid := <-pids:
			require.Contains(t, expectedPIDs, pid)
		}
	}
}

//nolint:tparallel // Subtests must be sequential, each starts its own reaper.
func TestForkReapExitCodes(t *testing.T) {
	t.Parallel()
	if !runSubprocess(t) {
		return
	}

	tests := []struct {
		name         string
		command      string
		expectedCode int
	}{
		{"exit 0", "exit 0", 0},
		{"exit 1", "exit 1", 1},
		{"exit 42", "exit 42", 42},
		{"exit 255", "exit 255", 255},
		{"SIGKILL", "kill -9 $$", 128 + 9},
		{"SIGTERM", "kill -15 $$", 128 + 15},
	}

	//nolint:paralleltest // Subtests must be sequential, each starts its own reaper.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reapLock sync.RWMutex
			opts := append([]reaper.Option{
				reaper.WithExecArgs("/bin/sh", "-c", tt.command),
				reaper.WithReapLock(&reapLock),
			}, withDone(t)...)
			reapLock.RLock()
			exitCode, err := reaper.ForkReap(opts...)
			reapLock.RUnlock()
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, exitCode, "exit code mismatch for %q", tt.command)
		})
	}
}

// TestReapInterrupt verifies that ForkReap forwards caught signals
// to the child process. The test sends SIGINT to its own process
// and checks that the child receives it. Running in a subprocess
// ensures SIGINT cannot kill the parent test binary.
func TestReapInterrupt(t *testing.T) {
	t.Parallel()
	if !runSubprocess(t) {
		return
	}

	errC := make(chan error, 1)
	pids := make(reap.PidCh, 1)

	// Use signals to notify when the child process is ready for the
	//  next step of our test.
	usrSig := make(chan os.Signal, 1)
	signal.Notify(usrSig, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(usrSig)

	go func() {
		opts := append([]reaper.Option{
			reaper.WithPIDCallback(pids),
			reaper.WithCatchSignals(os.Interrupt),
			// Signal propagation does not extend to children of children, so
			// we create a little bash script to ensure sleep is interrupted.
			reaper.WithExecArgs("/bin/sh", "-c", fmt.Sprintf(
				"pid=0; trap 'kill -USR2 %d; kill -TERM $pid' INT; sleep 10 &\npid=$!; kill -USR1 %d; wait",
				os.Getpid(), os.Getpid(),
			)),
		}, withDone(t)...)
		exitCode, err := reaper.ForkReap(opts...)
		// The child exits with 128 + SIGTERM (15) = 143, but the trap catches
		// SIGINT and sends SIGTERM to the sleep process, so exit code varies.
		_ = exitCode
		errC <- err
	}()

	require.Equal(t, syscall.SIGUSR1, <-usrSig)

	err := syscall.Kill(os.Getpid(), syscall.SIGINT)
	require.NoError(t, err)

	require.Equal(t, syscall.SIGUSR2, <-usrSig)
	require.NoError(t, <-errC)
}
