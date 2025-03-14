//go:build linux
// +build linux
package agentexec_test
import (
	"errors"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)
//nolint:paralleltest // This test is sensitive to environment variables
func TestCLI(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := cmd(ctx, t, 123, 12)
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()
		waitForSentinel(ctx, t, cmd, path)
		requireOOMScore(t, cmd.Process.Pid, 123)
		requireNiceScore(t, cmd.Process.Pid, 12)
	})
	t.Run("FiltersEnv", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := cmd(ctx, t, 123, 12)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=true", agentexec.EnvProcPrioMgmt))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=123", agentexec.EnvProcOOMScore))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=12", agentexec.EnvProcNiceScore))
		// Ensure unrelated environment variables are preserved.
		cmd.Env = append(cmd.Env, "CODER_TEST_ME_AGENTEXEC=true")
		err := cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()
		waitForSentinel(ctx, t, cmd, path)
		env := procEnv(t, cmd.Process.Pid)
		hasExecEnvs := slices.ContainsFunc(
			env,
			func(e string) bool {
				return strings.HasPrefix(e, agentexec.EnvProcPrioMgmt) ||
					strings.HasPrefix(e, agentexec.EnvProcOOMScore) ||
					strings.HasPrefix(e, agentexec.EnvProcNiceScore)
			})
		require.False(t, hasExecEnvs, "expected environment variables to be filtered")
		userEnv := slices.Contains(env, "CODER_TEST_ME_AGENTEXEC=true")
		require.True(t, userEnv, "expected user environment variables to be preserved")
	})
	t.Run("Defaults", func(t *testing.T) {
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
	t.Run("Capabilities", func(t *testing.T) {
		testdir := filepath.Dir(TestBin)
		capDir := filepath.Join(testdir, "caps")
		err := os.Mkdir(capDir, 0o755)
		require.NoError(t, err)
		bin := buildBinary(capDir)
		// Try to set capabilities on the binary. This should work fine in CI but
		// it's possible some developers may be working in an environment where they don't have the necessary permissions.
		err = setCaps(t, bin, "cap_net_admin")
		if os.Getenv("CI") != "" {
			require.NoError(t, err)
		} else if err != nil {
			t.Skipf("unable to set capabilities for test: %v", err)
		}
		ctx := testutil.Context(t, testutil.WaitMedium)
		cmd, path := binCmd(ctx, t, bin, 123, 12)
		err = cmd.Start()
		require.NoError(t, err)
		go cmd.Wait()
		waitForSentinel(ctx, t, cmd, path)
		// This is what we're really testing, a binary with added capabilities requires setting dumpable.
		requireOOMScore(t, cmd.Process.Pid, 123)
		requireNiceScore(t, cmd.Process.Pid, 12)
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
func binCmd(ctx context.Context, t *testing.T, bin string, oom, nice int) (*exec.Cmd, string) {
	var (
		args = execArgs(oom, nice)
		dir  = t.TempDir()
		file = filepath.Join(dir, "sentinel")
	)
	args = append(args, "sh", "-c", fmt.Sprintf("touch %s && sleep 10m", file))
	//nolint:gosec
	cmd := exec.CommandContext(ctx, bin, args...)
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
func cmd(ctx context.Context, t *testing.T, oom, nice int) (*exec.Cmd, string) {
	return binCmd(ctx, t, TestBin, oom, nice)
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
// procEnv returns the environment variables for a given process.
func procEnv(t *testing.T, pid int) []string {
	t.Helper()
	env, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	require.NoError(t, err)
	return strings.Split(string(env), "\x00")
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
func setCaps(t *testing.T, bin string, caps ...string) error {
	t.Helper()
	setcap := fmt.Sprintf("sudo -n setcap %s=ep %s", strings.Join(caps, ", "), bin)
	out, err := exec.CommandContext(context.Background(), "sh", "-c", setcap).CombinedOutput()
	if err != nil {
		return fmt.Errorf("setcap %q (%s): %w", setcap, out, err)
	}
	return nil
}
