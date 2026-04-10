//go:build linux

package terraform

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestProcessGroupCreation(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = cmdSysProcAttr()
	err := cmd.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid

	// The kernel exposes the process group ID as field 5 (index 4,
	// 1-based) in /proc/<pid>/stat.  When Setpgid is true the PGID
	// must equal the PID, meaning the process leads its own group.
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	require.NoError(t, err)

	fields := strings.Fields(string(data))
	require.True(t, len(fields) > 4, "unexpected stat format")

	pgid, err := strconv.Atoi(fields[4])
	require.NoError(t, err)
	require.Equal(t, pid, pgid,
		"PGID should equal PID when Setpgid is set")
}

func TestProcessGroupSignaling(t *testing.T) {
	t.Parallel()

	_ = testutil.Context(t, testutil.WaitShort)

	pidFile := t.TempDir() + "/pids"

	// The script records its own PID and the PID of a background
	// child.  Both must die when we signal the process group.
	script := t.TempDir() + "/test.sh"
	err := os.WriteFile(script, []byte(`#!/bin/bash
echo "parent_pid:$$" > "$1"
sleep 60 &
CHILD=$!
echo "child_pid:$CHILD" >> "$1"
# Keep the parent alive so the group leader exists.
sleep 60
`), 0o600)
	require.NoError(t, err)
	err = os.Chmod(script, 0o755)
	require.NoError(t, err)

	cmd := exec.Command("bash", script, pidFile)
	cmd.SysProcAttr = cmdSysProcAttr()
	err = cmd.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		// Belt-and-suspenders: make sure nothing lingers.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	// Wait for the PID file to contain both lines.
	var parentPID, childPID int
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) < 2 {
			return false
		}
		for _, line := range lines {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			pid, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			switch parts[0] {
			case "parent_pid":
				parentPID = pid
			case "child_pid":
				childPID = pid
			}
		}
		return parentPID != 0 && childPID != 0
	}, testutil.WaitShort, testutil.IntervalFast, "timed out waiting for PID file")

	// Use SIGKILL because bash ignores SIGINT for background
	// children started with &, and SIGTERM can also be caught.
	// SIGKILL cannot be caught and validates the group-signaling
	// mechanism reliably.
	err = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
	require.NoError(t, err)

	// Reap the parent via Wait() so it moves out of zombie state.
	// This also ensures cmd.Wait() returns, confirming the signal
	// actually reached the process group leader.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()
	select {
	case waitErr := <-waitDone:
		require.Error(t, waitErr, "process should have been killed")
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for parent to exit")
	}

	// After the parent is reaped, the child (an orphan adopted by
	// init) should also be gone.  If it's a zombie its /proc entry
	// will disappear once init reaps it, so we poll the /proc dir.
	require.Eventually(t, func() bool {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", childPID))
		return os.IsNotExist(err)
	}, testutil.WaitShort, testutil.IntervalFast, "child process still alive")
}
