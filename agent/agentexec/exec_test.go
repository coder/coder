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

//nolint:paralleltest // we need to test environment variables
func TestExec(t *testing.T) {
	//nolint:paralleltest // we need to test environment variables
	t.Run("NonLinux", func(t *testing.T) {
		t.Setenv(agentexec.EnvProcPrioMgmt, "true")

		if runtime.GOOS == "linux" {
			t.Skip("skipping on linux")
		}

		cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
		require.NoError(t, err)

		path, err := exec.LookPath("sh")
		require.NoError(t, err)
		require.Equal(t, path, cmd.Path)
		require.Equal(t, []string{"sh", "-c", "sleep"}, cmd.Args)
	})

	//nolint:paralleltest // we need to test environment variables
	t.Run("Linux", func(t *testing.T) {
		//nolint:paralleltest // we need to test environment variables
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

		//nolint:paralleltest // we need to test environment variables
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
			require.Equal(t, []string{executable, "agent-exec", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("Nice", func(t *testing.T) {
			t.Setenv(agentexec.EnvProcPrioMgmt, "hello")
			t.Setenv(agentexec.EnvProcNiceScore, "10")

			if runtime.GOOS != "linux" {
				t.Skip("skipping on linux")
			}

			executable, err := os.Executable()
			require.NoError(t, err)

			cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.NoError(t, err)
			require.Equal(t, executable, cmd.Path)
			require.Equal(t, []string{executable, "agent-exec", "--coder-nice=10", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("OOM", func(t *testing.T) {
			t.Setenv(agentexec.EnvProcPrioMgmt, "hello")
			t.Setenv(agentexec.EnvProcOOMScore, "123")

			if runtime.GOOS != "linux" {
				t.Skip("skipping on linux")
			}

			executable, err := os.Executable()
			require.NoError(t, err)

			cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.NoError(t, err)
			require.Equal(t, executable, cmd.Path)
			require.Equal(t, []string{executable, "agent-exec", "--coder-oom=123", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("Both", func(t *testing.T) {
			t.Setenv(agentexec.EnvProcPrioMgmt, "hello")
			t.Setenv(agentexec.EnvProcOOMScore, "432")
			t.Setenv(agentexec.EnvProcNiceScore, "14")

			if runtime.GOOS != "linux" {
				t.Skip("skipping on linux")
			}

			executable, err := os.Executable()
			require.NoError(t, err)

			cmd, err := agentexec.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.NoError(t, err)
			require.Equal(t, executable, cmd.Path)
			require.Equal(t, []string{executable, "agent-exec", "--coder-oom=432", "--coder-nice=14", "--", "sh", "-c", "sleep"}, cmd.Args)
		})
	})
}
