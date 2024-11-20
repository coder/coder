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
		cmd, path := cmd(ctx, t)
		cmd.Env = append(cmd.Env, "CODER_PROC_NICE_SCORE=10")
		cmd.Env = append(cmd.Env, "CODER_PROC_OOM_SCORE=123")
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()

		waitForSentinel(ctx, t, cmd, path)
		requireOOMScore(t, cmd.Process.Pid, 123)
		requireNiceScore(t, cmd.Process.Pid, 10)
	})

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := cmd(ctx, t)
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()

		waitForSentinel(ctx, t, cmd, path)

		expectedNice := expectedNiceScore(t)
		expectedOOM := expectedOOMScore(t)
		fmt.Println("expected nice", expectedNice, "expected oom", expectedOOM)
		requireOOMScore(t, cmd.Process.Pid, expectedOOM)
		requireNiceScore(t, cmd.Process.Pid, expectedNice)
	})
}

func requireNiceScore(t *testing.T, pid int, score int) {
	t.Helper()

	nice, err := unix.Getpriority(unix.PRIO_PROCESS, pid)
	require.NoError(t, err)
	require.Equal(t, score, nice)
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

func cmd(ctx context.Context, t *testing.T, args ...string) (*exec.Cmd, string) {
	file := ""
	//nolint:gosec
	cmd := exec.Command(TestBin, append([]string{"agent-exec"}, args...)...)
	if len(args) == 0 {
		// Generate a unique path that we can touch to indicate that we've progressed past the
		// syscall.Exec.
		dir := t.TempDir()
		file = filepath.Join(dir, "sentinel")
		//nolint:gosec
		cmd = exec.CommandContext(ctx, TestBin, "agent-exec", "sh", "-c", fmt.Sprintf("touch %s && sleep 10m", file))
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

		// if cmd.Process != nil {
		// 	_ = cmd.Process.Kill()
		// }
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
	score -= 20
	score += 5
	if score > 19 {
		return 19
	}
	return score
}
