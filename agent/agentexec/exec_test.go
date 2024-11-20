package agentexec_test

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentexec"
)

func TestExec(t *testing.T) {
	t.Run("NonLinux", func(t *testing.T) {

		t.Setenv(agentexec.EnvProcPrioMgmt, "true")

		if runtime.GOOS == "linux" {
			t.Skip("skipping on linux")
		}

		cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
		require.NoError(t, err)
		require.Equal(t, "sh", cmd.Path)
		require.Equal(t, []string{"-c", "sleep"}, cmd.Args[1:])
	})

	t.Run("Linux", func(t *testing.T) {

		t.Run("Disabled", func(t *testing.T) {
			if runtime.GOOS != "linux" {
				t.Skip("skipping on linux")
			}

			cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.NoError(t, err)
			path, err := exec.LookPath("sh")
			require.NoError(t, err)
			require.Equal(t, path, cmd.Path)
			require.Equal(t, []string{"sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("Enabled", func(t *testing.T) {
			t.Setenv(agentexec.EnvProcPrioMgmt, "hello")

			if runtime.GOOS != "linux" {
				t.Skip("skipping on linux")
			}

			executable, err := os.Executable()
			require.NoError(t, err)

			cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.NoError(t, err)
			require.Equal(t, executable, cmd.Path)
			require.Equal(t, []string{executable, "agent-exec", "sh", "-c", "sleep"}, cmd.Args)
		})
	})
}
