//go:build !windows

package reaper_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/go-reap"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/agent/reaper"
)

func TestReap(t *testing.T) {
	t.Parallel()

	// Because we're forkexecing these tests will try to run twice...
	if reaper.IsChild() {
		t.Skip("I'm a child!")
	}

	// OK checks that's the reaper is successfully reaping
	// exited processes and passing the PIDs through the shared
	// channel.
	t.Run("OK", func(t *testing.T) {
		pids := make(reap.PidCh, 1)
		err := reaper.ForkReap(pids)
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

		deadline := time.NewTimer(time.Second * 5)
		for i := 0; i < len(expectedPIDs); i++ {
			select {
			case <-deadline.C:
				t.Fatalf("Timed out waiting for process")
			case pid := <-pids:
				require.Contains(t, expectedPIDs, pid)
			}
		}
	})
}
