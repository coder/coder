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

	t.Run("OK", func(t *testing.T) {
		pids := make(reap.PidCh, 1)
		err := reaper.ForkReap(pids)
		require.NoError(t, err)

		cmd := exec.Command("sleep", "5")
		err = cmd.Start()
		require.NoError(t, err)

		cmd2 := exec.Command("sleep", "5")
		err = cmd2.Start()
		require.NoError(t, err)

		err = cmd.Process.Kill()
		require.NoError(t, err)

		err = cmd2.Process.Kill()
		require.NoError(t, err)

		expectedPIDs := []int{cmd.Process.Pid, cmd2.Process.Pid}

		deadline := time.NewTimer(time.Second * 5)
		select {
		case <-deadline.C:
			t.Fatalf("Timed out waiting for process")
		case pid := <-pids:
			require.Contains(t, expectedPIDs, pid)
		}
	})
}
