//go:build linux

package reaper_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/go-reap"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent/reaper"
	"github.com/coder/coder/testutil"
)

func TestReap(t *testing.T) {
	t.Parallel()

	// Don't run the reaper test in CI. It does weird
	// things like forkexecing which may have unintended
	// consequences in CI.
	if _, ok := os.LookupEnv("CI"); ok {
		t.Skip("Detected CI, skipping reaper tests")
	}

	// OK checks that's the reaper is successfully reaping
	// exited processes and passing the PIDs through the shared
	// channel.
	t.Run("OK", func(t *testing.T) {
		t.Parallel()
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
