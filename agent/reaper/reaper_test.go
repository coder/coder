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

	"github.com/coder/coder/agent/reaper"
	"github.com/coder/coder/testutil"
)

//nolint:paralleltest // Non-parallel subtest.
func TestReap(t *testing.T) {
	// Don't run the reaper test in CI. It does weird
	// things like forkexecing which may have unintended
	// consequences in CI.
	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("Detected CI, skipping reaper tests")
	}

	// OK checks that's the reaper is successfully reaping
	// exited processes and passing the PIDs through the shared
	// channel.

	//nolint:paralleltest // Signal handling.
	t.Run("OK", func(t *testing.T) {
		pids := make(reap.PidCh, 1)
		err := reaper.ForkReap(
			reaper.WithPIDCallback(pids),
			// Provide some argument that immediately exits.
			reaper.WithExecArgs("/bin/sh", "-c", "exit 0"),
		)
		require.NoError(t, err)

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
	})
}

//nolint:paralleltest // Signal handling.
func TestReapInterrupt(t *testing.T) {
	// Don't run the reaper test in CI. It does weird
	// things like forkexecing which may have unintended
	// consequences in CI.
	if _, ok := os.LookupEnv("CI"); ok {
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
		errC <- reaper.ForkReap(
			reaper.WithPIDCallback(pids),
			reaper.WithCatchSignals(os.Interrupt),
			// Signal propagation does not extend to children of children, so
			// we create a little bash script to ensure sleep is interrupted.
			reaper.WithExecArgs("/bin/sh", "-c", fmt.Sprintf("pid=0; trap 'kill -USR2 %d; kill -TERM $pid' INT; sleep 10 &\npid=$!; kill -USR1 %d; wait", os.Getpid(), os.Getpid())),
		)
	}()

	require.Equal(t, <-usrSig, syscall.SIGUSR1)
	err := syscall.Kill(os.Getpid(), syscall.SIGINT)
	require.NoError(t, err)
	require.Equal(t, <-usrSig, syscall.SIGUSR2)

	require.NoError(t, <-errC)
}
