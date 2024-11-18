package agentexec_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/v2/testutil"
)

func TestCLI(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, dir := cmd(t, ctx)
		cmd.Env = append(cmd.Env, "CODER_PROC_NICE_SCORE=10")
		cmd.Env = append(cmd.Env, "CODER_PROC_OOM_SCORE=123")
		err := cmd.Start()
		require.NoError(t, err)

		waitForSentinel(t, ctx, cmd, dir)
		requireOOMScore(t, cmd.Process.Pid, 123)
		requireNiceScore(t, cmd.Process.Pid, 10)
	})
}

func requireNiceScore(t *testing.T, pid int, score int) {
	t.Helper()

	nice, err := unix.Getpriority(0, pid)
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

func waitForSentinel(t *testing.T, ctx context.Context, cmd *exec.Cmd, dir string) {
	t.Helper()

	require.Eventually(t, func() bool {
		// Check if the process is still running.
		err := cmd.Process.Signal(syscall.Signal(0))
		require.NoError(t, err)

		_, err = os.Stat(dir)
		return err == nil && ctx.Err() == nil
	}, testutil.WaitLong, testutil.IntervalFast)
}

func cmd(t *testing.T, ctx context.Context, args ...string) (*exec.Cmd, string) {
	dir := ""
	cmd := exec.Command(TestBin, args...)
	if len(args) == 0 {
		dir = t.TempDir()
		//nolint:gosec
		cmd = exec.CommandContext(ctx, TestBin, "sh", "-c", fmt.Sprintf("touch %s && sleep 10m", dir))
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	t.Cleanup(func() {
		// Print output of a command if the test fails.
		if t.Failed() {
			t.Logf("cmd %q output: %s", cmd.Args, buf.String())
		}

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return cmd, dir
}
