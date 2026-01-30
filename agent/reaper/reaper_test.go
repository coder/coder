//go:build linux

package reaper_test

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/go-reap"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/reaper"
	"github.com/coder/coder/v2/testutil"
)

// TestReap checks that's the reaper is successfully reaping
// exited processes and passing the PIDs through the shared
// channel.
//
//nolint:paralleltest
func TestReap(t *testing.T) {
	// Don't run the reaper test in CI. It does weird
	// things like forkexecing which may have unintended
	// consequences in CI.
	if testutil.InCI() {
		t.Skip("Detected CI, skipping reaper tests")
	}

	pids := make(reap.PidCh, 1)
	exitCode, err := reaper.ForkReap(
		reaper.WithPIDCallback(pids),
		// Provide some argument that immediately exits.
		reaper.WithExecArgs("/bin/sh", "-c", "exit 0"),
	)
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

	for i := 0; i < len(expectedPIDs); i++ {
		select {
		case <-time.After(testutil.WaitShort):
			t.Fatalf("Timed out waiting for process")
		case pid := <-pids:
			require.Contains(t, expectedPIDs, pid)
		}
	}
}

//nolint:paralleltest
func TestForkReapExitCodes(t *testing.T) {
	if testutil.InCI() {
		t.Skip("Detected CI, skipping reaper tests")
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode, err := reaper.ForkReap(
				reaper.WithExecArgs("/bin/sh", "-c", tt.command),
			)
			require.NoError(t, err)
			require.Equal(t, tt.expectedCode, exitCode, "exit code mismatch for %q", tt.command)
		})
	}
}

//nolint:paralleltest // Signal handling.
func TestReapInterrupt(t *testing.T) {
	// Don't run the reaper test in CI. It does weird
	// things like forkexecing which may have unintended
	// consequences in CI.
	if testutil.InCI() {
		t.Skip("Detected CI, skipping reaper tests")
	}

	errC := make(chan error, 1)
	pids := make(reap.PidCh, 1)

	// Use signals to notify when the child process is ready for the
	//  next step of our test.
	usrSig := make(chan os.Signal, 1)
	signal.Notify(usrSig, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(usrSig)

	go func() {
		exitCode, err := reaper.ForkReap(
			reaper.WithPIDCallback(pids),
			reaper.WithCatchSignals(os.Interrupt),
			// Signal propagation does not extend to children of children, so
			// we create a little bash script to ensure sleep is interrupted.
			reaper.WithExecArgs("/bin/sh", "-c", fmt.Sprintf("pid=0; trap 'kill -USR2 %d; kill -TERM $pid' INT; sleep 10 &\npid=$!; kill -USR1 %d; wait", os.Getpid(), os.Getpid())),
		)
		// The child exits with 128 + SIGTERM (15) = 143, but the trap catches
		// SIGINT and sends SIGTERM to the sleep process, so exit code varies.
		_ = exitCode
		errC <- err
	}()

	require.Equal(t, <-usrSig, syscall.SIGUSR1)
	err := syscall.Kill(os.Getpid(), syscall.SIGINT)
	require.NoError(t, err)
	require.Equal(t, <-usrSig, syscall.SIGUSR2)

	require.NoError(t, <-errC)
}
