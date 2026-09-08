//go:build linux

package xvnc_test

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentdesktop/desktopruntime"
	"github.com/coder/coder/v2/agent/x/agentdesktop/xvnc"
	"github.com/coder/coder/v2/testutil"
)

// TestStart exercises the real embedded Xvnc. The runtime module only ships
// archives for linux amd64 and arm64.
func TestStart(t *testing.T) {
	t.Parallel()
	if !desktopruntime.Available() {
		t.Skip("desktop runtime not available on this platform")
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	runtimeDir, err := desktopruntime.Unpack(ctx, t.TempDir())
	require.NoError(t, err)

	logFile, err := os.CreateTemp(t.TempDir(), "xvnc-*.log")
	require.NoError(t, err)
	defer logFile.Close()

	srv, err := xvnc.Start(ctx, xvnc.Options{
		RuntimeDir: runtimeDir,
		Width:      640,
		Height:     480,
		Log:        logFile,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Stop(5 * time.Second) })

	require.GreaterOrEqual(t, srv.Display, 10)
	require.NotZero(t, srv.Port)

	// The server must answer with an RFB banner and accept a second client
	// while the first is still connected (AlwaysShared).
	first := dialRFB(t, srv.Addr())
	defer first.Close()
	second := dialRFB(t, srv.Addr())
	defer second.Close()

	// The X server must have compiled its keymap from the bundled data.
	logBytes, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	log := string(logBytes)
	require.NotContains(t, log, "Failed to compile keymap")
	require.NotContains(t, log, "Could not load keymap")
	require.NotContains(t, log, "Fatal server error")

	require.NoError(t, srv.Stop(5*time.Second))
	<-srv.Done()
	// Xvnc exits non-zero on SIGTERM, which is expected.
	_, dialErr := net.DialTimeout("tcp", srv.Addr(), time.Second)
	require.Error(t, dialErr, "port should be closed after Stop")
}

// TestStartInvalidRuntime verifies validation happens before anything is
// launched.
func TestStartInvalidRuntime(t *testing.T) {
	t.Parallel()
	_, err := xvnc.Start(context.Background(), xvnc.Options{RuntimeDir: t.TempDir()})
	require.ErrorContains(t, err, "invalid runtime dir")
}

// TestStartExitsWithParent verifies Xvnc does not outlive a caller that dies
// without calling Stop. The launcher sets Pdeathsig, so killing the process
// that started Xvnc must also end Xvnc.
func TestStartExitsWithParent(t *testing.T) {
	t.Parallel()
	if !desktopruntime.Available() {
		t.Skip("desktop runtime not available on this platform")
	}
	if os.Getenv("CODER_TEST_XVNC_CHILD") == "1" {
		// Child mode: start Xvnc, report its pid and block forever.
		runtimeDir, err := desktopruntime.Unpack(context.Background(), os.Getenv("CODER_TEST_XVNC_CACHE"))
		if err != nil {
			t.Fatal(err)
		}
		srv, err := xvnc.Start(context.Background(), xvnc.Options{RuntimeDir: runtimeDir})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = os.Stdout.WriteString(strconv.Itoa(srv.Cmd.Process.Pid) + "\n")
		select {}
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	cacheDir := t.TempDir()
	// Unpack once here so the child does not race the test timeout.
	_, err := desktopruntime.Unpack(ctx, cacheDir)
	require.NoError(t, err)

	//nolint:gosec // re-executing the test binary is the standard pattern.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartExitsWithParent$", "-test.v")
	cmd.Env = append(os.Environ(),
		"CODER_TEST_XVNC_CHILD=1",
		"CODER_TEST_XVNC_CACHE="+cacheDir,
	)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	var xvncPID int
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if pid, perr := strconv.Atoi(line); perr == nil && pid > 0 {
			xvncPID = pid
			break
		}
	}
	require.NotZero(t, xvncPID, "child never reported the Xvnc pid")
	require.NoError(t, syscall.Kill(xvncPID, 0), "Xvnc should be running")

	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()

	require.Eventually(t, func() bool {
		// ESRCH means the process is gone. A zombie still answers signal 0,
		// so also check /proc state.
		if err := syscall.Kill(xvncPID, 0); err != nil {
			return true
		}
		state, err := os.ReadFile("/proc/" + strconv.Itoa(xvncPID) + "/stat")
		return err != nil || strings.Contains(string(state), ") Z ")
	}, testutil.WaitLong, testutil.IntervalMedium, "Xvnc outlived its parent")
}

func dialRFB(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	banner, err := bufio.NewReader(conn).ReadString('\n')
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(banner, "RFB 003."), "unexpected banner %q", banner)
	return conn
}
