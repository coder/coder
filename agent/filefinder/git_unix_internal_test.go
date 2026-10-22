//go:build !windows

package filefinder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)

// detachedExecer starts a child in its own session, outside git's
// process group, that holds only stdout and records its PID.
type detachedExecer struct {
	agentexec.Execer
	pidFile string
}

func (e detachedExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	//nolint:gosec // Test-controlled arguments.
	return exec.CommandContext(ctx, "sh", "-c",
		`setsid sh -c 'echo $$ > "$0"; exec sleep 60' "$1" 2>/dev/null </dev/null & sleep 60`, "sh", e.pidFile)
}

func TestGitRunner_CancelUnblocksDetachedStdout(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	g := newGitRunner(detachedExecer{pidFile: pidFile}, "sh")
	require.NotNil(t, g)
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- g.run(runCtx, time.Minute, t.TempDir(), nil, func(string) error { return nil }, "ls-files")
	}()
	// Cancel once the detached child is running and holding stdout.
	require.Eventually(t, func() bool {
		_, err := os.Stat(pidFile)
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast)
	cancel()
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)
}
