//go:build linux

package agentnative

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

func codexStartTestServer(t *testing.T, c *Codex) {
	t.Helper()
	clock := c.clock
	c.clock = quartz.NewReal()
	defer func() { c.clock = clock }()
	require.NoError(t, c.startServer())
}

func TestCodexServerOwnershipAndPassiveClose(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	c := newCodexFixture(t).codex
	dial := c.dial
	attempts := 0
	c.dial = func(ctx context.Context, onMessage func(codexMessage), onDisconnect func()) (codexRPC, error) {
		attempts++
		if attempts == 1 {
			return nil, xerrors.New("no server")
		}
		return dial(ctx, onMessage, onDisconnect)
	}
	var child *exec.Cmd
	c.options.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.Equal(t, "codex", name)
		require.Equal(t, []string{"app-server", "--listen", "unix://" + filepath.Join(c.options.StateDir, "codex.sock")}, args)
		child = exec.CommandContext(ctx, "sleep", "3600")
		return child
	}
	t.Cleanup(func() {
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
		}
	})
	clock := c.clock
	c.clock = quartz.NewReal()
	c.lifecycle.Lock()
	connection, err := c.ensure(ctx, true)
	c.lifecycle.Unlock()
	c.clock = clock
	require.NoError(t, err)
	require.NotNil(t, connection)
	require.Equal(t, 2, attempts)
	require.NotNil(t, child)
	require.Same(t, child, c.owned)
	require.NoError(t, c.Close())
	require.NoError(t, child.Process.Signal(syscall.Signal(0)), "passive close must leave the native process running")
	exited := c.ownedDone
	require.NoError(t, c.Shutdown(ctx))
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.ErrorIs(t, child.Process.Signal(syscall.Signal(0)), os.ErrProcessDone)
	log, err := os.Stat(filepath.Join(c.options.StateDir, "codex-server.log"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), log.Mode().Perm())
}

// The launcher dies on SIGTERM while its native child deliberately keeps
// draining. This reproduces npm wrappers without depending on npm or Codex.
//
//nolint:paralleltest,revive // Subprocess mode owns signal handlers and deliberately exits without the test runner.
func TestCodexProcessHelper(t *testing.T) {
	mode := os.Getenv("CODEX_PROCESS_HELPER")
	if mode == "" {
		t.Parallel()
		return
	}
	ready := os.NewFile(3, "ready")
	if mode == "listener" {
		listener, err := net.Listen("unix", os.Getenv("CODEX_TEST_SOCKET"))
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprintln(ready, os.Getpid())
		_ = ready.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				os.Exit(2)
			}
			_ = conn.Close()
		}
	}
	if mode == "launcher" || mode == "restricted-launcher" {
		// #nosec G204 -- Reexecute only this test binary in native-child mode.
		child := exec.Command(os.Args[0], "-test.run=^TestCodexProcessHelper$")
		child.Env = append(os.Environ(), "CODEX_PROCESS_HELPER=native")
		if mode == "restricted-launcher" {
			child.Env = []string{"CODEX_PROCESS_HELPER=native"}
			child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}
		child.ExtraFiles = []*os.File{ready}
		if child.Start() != nil {
			os.Exit(2)
		}
		_ = ready.Close()
		_ = child.Wait()
		os.Exit(0)
	}
	signal.Ignore(syscall.SIGTERM)
	_, _ = fmt.Fprintf(ready, "%d\n", os.Getpid())
	_ = ready.Close()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGUSR2)
	<-stop
	os.Exit(0)
}

func TestCodexRestartShutdownEscalatesSurvivingNativeChild(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	c, host, _, _, clock := newCodexTest(t)
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	c.options.Environment = append(c.options.Environment, "CODEX_PROCESS_HELPER=launcher")
	c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		// #nosec G204 -- Reexecute only this test binary in launcher mode.
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexProcessHelper$")
		cmd.ExtraFiles = []*os.File{writer}
		return cmd
	}
	codexStartTestServer(t, c)
	t.Cleanup(func() { _ = c.killOwnedGroup() })
	_ = writer.Close()
	line, err := bufio.NewReader(reader).ReadString('\n')
	require.NoError(t, err)
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	childFD, err := unix.PidfdOpen(childPID, 0)
	require.NoError(t, err)
	defer unix.Close(childFD)
	launcherExited := c.ownedDone
	require.NoError(t, c.Close())
	reopened, err := NewCodex(BackendOptions{StateDir: c.options.StateDir, Host: host, Clock: clock})
	require.NoError(t, err)
	defer reopened.Close()
	require.Nil(t, reopened.owned, "shutdown must use the durable ownership record")
	trap := clock.Trap().AfterFunc("codex-shutdown-wait")
	trapClosed := false
	defer func() {
		if !trapClosed {
			trap.Close()
		}
	}()
	result := make(chan error, 1)
	go func() { result <- reopened.Shutdown(ctx) }()
	first := trap.MustWait(ctx)
	first.MustRelease(ctx)
	select {
	case <-launcherExited:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second := trap.MustWait(ctx)
	second.MustRelease(ctx)
	trap.Close()
	trapClosed = true
	pollFD, err := codexPollDescriptor(childFD)
	require.NoError(t, err)
	poll := []unix.PollFd{pollFD}
	count, err := unix.Poll(poll, 0)
	require.NoError(t, err)
	require.Zero(t, count, "native child must survive launcher termination")
	clock.Advance(5 * time.Second).MustWait(ctx)
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	count, err = unix.Poll(poll, 0)
	require.NoError(t, err)
	require.Equal(t, 1, count, "native child must be dead after escalation")
	require.NoError(t, reopened.Shutdown(ctx), "durable dead-owner tombstone permits idempotent shutdown")
}

func TestCodexShutdownRefusesChangedProcessIdentity(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"start", "executable", "marker", "boot"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			c, host, _, _, clock := newCodexTest(t)
			c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "sleep", "3600")
			}
			codexStartTestServer(t, c)
			t.Cleanup(func() { _ = c.killOwnedGroup() })
			record, err := c.readServerRecord()
			require.NoError(t, err)
			switch field {
			case "start":
				record.Leader.Start = "1"
			case "executable":
				record.Leader.Executable += "-other"
			case "marker":
				record.Marker = "00000000-0000-0000-0000-000000000000"
			case "boot":
				record.Boot = "different-boot"
			}
			require.NoError(t, c.writeServerRecord(*record))
			reopened, err := NewCodex(BackendOptions{StateDir: c.options.StateDir, Host: host, Clock: clock})
			require.NoError(t, err)
			defer reopened.Close()
			require.ErrorIs(t, reopened.Shutdown(t.Context()), errCodexOwnership)
			require.NoError(t, c.owned.Process.Signal(syscall.Signal(0)))
		})
	}
}

func TestCodexOwnedStartupFinalizesExecutableIdentity(t *testing.T) {
	t.Parallel()
	c := newCodexFixture(t).codex
	c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "3600")
	}
	codexStartTestServer(t, c)
	t.Cleanup(func() { _ = c.killOwnedGroup() })
	actual := *c.ownedRecord
	// Model the short-lived agent-exec image captured before native readiness.
	c.ownedRecord.Leader.Executable = "/bootstrap/agent-exec"
	require.NoError(t, c.writeServerRecord(*c.ownedRecord))
	require.NoError(t, c.finalizeServerIdentity())
	require.Equal(t, actual, *c.ownedRecord)
	durable, err := c.readServerRecord()
	require.NoError(t, err)
	require.Equal(t, actual, *durable)
}

func TestCodexShutdownRejectsLostOwnershipDirectory(t *testing.T) {
	t.Parallel()
	c := newCodexFixture(t).codex
	c.seenServer = true
	require.NoError(t, os.Remove(c.options.StateDir))
	require.ErrorIs(t, c.Shutdown(t.Context()), errCodexOwnership)
}

func TestCodexAttachDoesNotStartMissingServer(t *testing.T) {
	t.Parallel()
	for _, server := range []string{"existing", "missing"} {
		t.Run(server, func(t *testing.T) {
			t.Parallel()
			f := newCodexFixture(t)
			c, s := f.codex, f.session
			home := t.TempDir()
			codexHome := filepath.Join(home, ".codex")
			c.options.Environment = []string{"HOME=" + home, "CODEX_HOME=" + codexHome}
			if server == "missing" {
				c.dial = func(context.Context, func(codexMessage), func()) (codexRPC, error) {
					return nil, xerrors.New("no server")
				}
			}
			_, err := c.Attach(t.Context(), s)
			if server == "missing" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, c.owned)
			require.NoDirExists(t, codexHome, "read-only attachment must not prepare a native home")
		})
	}
}

func TestCodexServerRejectsUnsafeLog(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"symlink", "permissions", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			c := newCodexFixture(t).codex
			log := filepath.Join(c.options.StateDir, "codex-server.log")
			target := filepath.Join(c.options.StateDir, "target")
			require.NoError(t, os.WriteFile(target, []byte("preserve"), 0o600))
			switch scenario {
			case "symlink":
				require.NoError(t, os.Symlink(target, log))
			case "hardlink":
				require.NoError(t, os.Link(target, log))
			case "permissions":
				// #nosec G306 -- Intentionally unsafe permissions exercise rejection.
				require.NoError(t, os.WriteFile(log, nil, 0o644))
			}
			require.Error(t, c.startServer())
			data, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, "preserve", string(data))
		})
	}
}
