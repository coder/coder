//go:build linux

package agentnative

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/quartz"
)

func codexShortState(t *testing.T, c *Codex) {
	t.Helper()
	dir, err := os.MkdirTemp("", "codex-life-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	c.options.StateDir = dir
}

func codexSleepOwner(t *testing.T) *Codex {
	t.Helper()
	c := newCodexFixture(t).codex
	codexShortState(t, c)
	c.clock = quartz.NewReal()
	c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sleep", "3600")
	}
	require.NoError(t, c.startServer())
	t.Cleanup(func() { _ = c.killOwnedGroup() })
	return c
}

func TestCodexRetiresClosedContainerDomain(t *testing.T) {
	t.Parallel()
	for _, socket := range []string{"absent", "closed", "live"} {
		t.Run(socket, func(t *testing.T) {
			t.Parallel()
			c := codexSleepOwner(t)
			require.NoError(t, c.Shutdown(codexTestContext(t)))
			record, err := c.readServerRecord()
			require.NoError(t, err)
			boot, namespace, err := codexProcessDomain()
			require.NoError(t, err)
			require.Equal(t, boot, record.Boot)
			record.Namespace = namespace + "-retired-container"
			// A same-numbered process in the new namespace must never be signaled.
			record.Leader.PID = os.Getpid()
			record.Leader.Group = os.Getpid()
			record.Leader.Session = os.Getpid()
			require.NoError(t, c.writeServerRecord(*record))
			if socket != "absent" {
				listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(c.options.StateDir, "codex.sock"), Net: "unix"})
				require.NoError(t, err)
				defer listener.Close()
				if socket == "closed" {
					listener.SetUnlinkOnClose(false)
					require.NoError(t, listener.Close())
				}
			}
			err = c.checkServerStart(codexTestContext(t))
			if socket == "live" {
				require.ErrorIs(t, err, errCodexOwnership)
			} else {
				require.NoError(t, err)
			}
			unchanged, err := c.readServerRecord()
			require.NoError(t, err)
			require.Equal(t, record, unchanged, "preflight must not erase history or rewrite ownership before a new launch")
		})
	}
}

func TestCodexRestrictedChildDoesNotBlockOwnerShutdown(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	c := newCodexFixture(t).codex
	c.clock = quartz.NewReal()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	c.options.Environment = append(c.options.Environment, "CODEX_PROCESS_HELPER=restricted-launcher")
	c.options.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		// #nosec G204 -- Reexecute only this test binary with the restricted child fixture.
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexProcessHelper$")
		cmd.ExtraFiles = []*os.File{writer}
		return cmd
	}
	require.NoError(t, c.startServer())
	t.Cleanup(func() { _ = c.killOwnedGroup() })
	_ = writer.Close()
	line, err := bufio.NewReader(reader).ReadString('\n')
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	fd, err := unix.PidfdOpen(pid, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); _ = unix.Close(fd) })
	identity, marker, err := codexProcessIdentity(pid)
	require.NoError(t, err)
	require.Empty(t, marker)
	require.Equal(t, c.ownedRecord.Leader.Session, identity.Session)
	require.NotEqual(t, c.ownedRecord.Leader.Group, identity.Group)
	exited := c.ownedDone
	require.NoError(t, c.Shutdown(ctx))
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, unix.PidfdSendSignal(fd, 0, nil, 0), "unverified MCP child must remain untouched")
}

func TestCodexRejectsReplacementListenerAfterOwnerExit(t *testing.T) {
	t.Parallel()
	ctx := codexTestContext(t)
	c := codexSleepOwner(t)
	require.NoError(t, c.Shutdown(ctx))
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	// #nosec G204 -- Reexecute only this test binary to provide an unrelated listener.
	replacement := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexProcessHelper$")
	replacement.Env = append(os.Environ(), "CODEX_PROCESS_HELPER=listener", "CODEX_TEST_SOCKET="+filepath.Join(c.options.StateDir, "codex.sock"))
	replacement.ExtraFiles = []*os.File{writer}
	replacement.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, replacement.Start())
	t.Cleanup(func() { _ = replacement.Process.Kill(); _ = replacement.Wait() })
	_ = writer.Close()
	_, err = bufio.NewReader(reader).ReadString('\n')
	require.NoError(t, err)
	reopened, err := NewCodex(BackendOptions{Host: c.options.Host, StateDir: c.options.StateDir})
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.ensure(ctx, true)
	require.ErrorIs(t, err, errCodexOwnership, "a dead-owner tombstone cannot authorize a replacement listener")
	require.ErrorIs(t, reopened.Shutdown(ctx), errCodexOwnership)
	require.NoError(t, replacement.Process.Signal(syscall.Signal(0)))
}
