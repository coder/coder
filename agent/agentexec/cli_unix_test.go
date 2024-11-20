//go:build linux
// +build linux

package agentexec_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/testutil"
)

func TestCLI(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := cmd(ctx, t, 123, 12)
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()

		waitForSentinel(ctx, t, cmd, path)
		requireOOMScore(t, cmd.Process.Pid, 123)
		requireNiceScore(t, cmd.Process.Pid, 12)
	})

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := cmd(ctx, t, 0, 0)
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()

		waitForSentinel(ctx, t, cmd, path)

		expectedNice := expectedNiceScore(t)
		expectedOOM := expectedOOMScore(t)
		requireOOMScore(t, cmd.Process.Pid, expectedOOM)
		requireNiceScore(t, cmd.Process.Pid, expectedNice)
	})
}

func requireNiceScore(t *testing.T, pid int, score int) {
	t.Helper()

	nice, err := unix.Getpriority(unix.PRIO_PROCESS, pid)
	require.NoError(t, err)
	// See https://linux.die.net/man/2/setpriority#Notes
	require.Equal(t, score, 20-nice)
}

func requireOOMScore(t *testing.T, pid int, expected int) {
	t.Helper()

	actual, err := os.ReadFile(fmt.Sprintf("/proc/%d/oom_score_adj", pid))
	require.NoError(t, err)
	score := strings.TrimSpace(string(actual))
	require.Equal(t, strconv.Itoa(expected), score)
}

func waitForSentinel(ctx context.Context, t *testing.T, cmd *exec.Cmd, path string) {
	t.Helper()

	ticker := time.NewTicker(testutil.IntervalFast)
	defer ticker.Stop()

	// RequireEventually doesn't work well with require.NoError or similar require functions.
	for {
		err := cmd.Process.Signal(syscall.Signal(0))
		require.NoError(t, err)

		_, err = os.Stat(path)
		if err == nil {
			return
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			require.NoError(t, ctx.Err())
		}
	}
}

func cmd(ctx context.Context, t *testing.T, oom, nice int) (*exec.Cmd, string) {
	var (
		args = execArgs(oom, nice)
		dir  = t.TempDir()
		file = filepath.Join(dir, "sentinel")
	)

	args = append(args, "sh", "-c", fmt.Sprintf("touch %s && sleep 10m", file))
	//nolint:gosec
	cmd := exec.CommandContext(ctx, TestBin, args...)

	// We set this so we can also easily kill the sleep process the shell spawns.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	t.Cleanup(func() {
		// Print output of a command if the test fails.
		if t.Failed() {
			t.Logf("cmd %q output: %s", cmd.Args, buf.String())
		}
		if cmd.Process != nil {
			// We use -cmd.Process.Pid to kill the whole process group.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		}
	})
	return cmd, file
}

func expectedOOMScore(t *testing.T) int {
	t.Helper()

	score, err := os.ReadFile(fmt.Sprintf("/proc/%d/oom_score_adj", os.Getpid()))
	require.NoError(t, err)

	scoreInt, err := strconv.Atoi(strings.TrimSpace(string(score)))
	require.NoError(t, err)

	if scoreInt < 0 {
		return 0
	}
	if scoreInt >= 998 {
		return 1000
	}
	return 998
}

func expectedNiceScore(t *testing.T) int {
	t.Helper()

	score, err := unix.Getpriority(unix.PRIO_PROCESS, os.Getpid())
	require.NoError(t, err)

	// Priority is niceness + 20.
	score = 20 - score
	score += 5
	if score > 19 {
		return 19
	}
	return score
}

func execArgs(oom int, nice int) []string {
	execArgs := []string{"agent-exec"}
	if oom != 0 {
		execArgs = append(execArgs, fmt.Sprintf("--coder-oom=%d", oom))
	}
	if nice != 0 {
		execArgs = append(execArgs, fmt.Sprintf("--coder-nice=%d", nice))
	}
	execArgs = append(execArgs, "--")
	return execArgs
}
