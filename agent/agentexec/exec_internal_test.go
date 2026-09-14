package agentexec

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecer(t *testing.T) {
	t.Parallel()

	t.Run("Default", func(t *testing.T) {
		t.Parallel()

		cmd := DefaultExecer.CommandContext(context.Background(), "sh", "-c", "sleep")

		path, err := exec.LookPath("sh")
		require.NoError(t, err)
		require.Equal(t, path, cmd.Path)
		require.Equal(t, []string{"sh", "-c", "sleep"}, cmd.Args)
	})

	t.Run("Priority", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()

			e := priorityExecer{
				binPath:   "/foo/bar/baz",
				oomScore:  unset,
				niceScore: unset,
			}

			cmd := e.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.Equal(t, e.binPath, cmd.Path)
			require.Equal(t, []string{e.binPath, "agent-exec", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("Nice", func(t *testing.T) {
			t.Parallel()

			e := priorityExecer{
				binPath:   "/foo/bar/baz",
				oomScore:  unset,
				niceScore: 10,
			}

			cmd := e.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.Equal(t, e.binPath, cmd.Path)
			require.Equal(t, []string{e.binPath, "agent-exec", "--coder-nice=10", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("OOM", func(t *testing.T) {
			t.Parallel()

			e := priorityExecer{
				binPath:   "/foo/bar/baz",
				oomScore:  123,
				niceScore: unset,
			}

			cmd := e.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.Equal(t, e.binPath, cmd.Path)
			require.Equal(t, []string{e.binPath, "agent-exec", "--coder-oom=123", "--", "sh", "-c", "sleep"}, cmd.Args)
		})

		t.Run("Both", func(t *testing.T) {
			t.Parallel()

			e := priorityExecer{
				binPath:   "/foo/bar/baz",
				oomScore:  432,
				niceScore: 14,
			}

			cmd := e.CommandContext(context.Background(), "sh", "-c", "sleep")
			require.Equal(t, e.binPath, cmd.Path)
			require.Equal(t, []string{e.binPath, "agent-exec", "--coder-oom=432", "--coder-nice=14", "--", "sh", "-c", "sleep"}, cmd.Args)
		})
	})
}
