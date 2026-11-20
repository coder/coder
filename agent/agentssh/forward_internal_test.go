//go:build !windows

package agentssh

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)

func TestUnixForwardListener_OlderSessionDoesNotUnlinkNewerSessionSocket(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "forward.sock")

	// Session 1 creates the listener and records its FileInfo.
	ln1, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer ln1.Close()

	fi1, err := os.Lstat(socketPath)
	require.NoError(t, err)

	ul1, ok := ln1.(*net.UnixListener)
	require.True(t, ok)
	ul1.SetUnlinkOnClose(false)

	uln1 := &unixForwardListener{
		Listener: ln1,
		path:     socketPath,
		fi:       fi1,
	}

	// Session 2 connects, unlinks the previous socket and creates a new one at the same path.
	err = unlink(socketPath)
	require.NoError(t, err)

	ln2, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer ln2.Close()

	fi2, err := os.Lstat(socketPath)
	require.NoError(t, err)
	require.False(t, os.SameFile(fi1, fi2), "new socket must have different identity")

	ul2, ok := ln2.(*net.UnixListener)
	require.True(t, ok)
	ul2.SetUnlinkOnClose(false)

	uln2 := &unixForwardListener{
		Listener: ln2,
		path:     socketPath,
		fi:       fi2,
	}

	// Session 1 closes: it must not unlink Session 2's socket.
	err = uln1.Close()
	require.NoError(t, err)

	// Session 2's socket must still exist on disk.
	fiCurrent, err := os.Lstat(socketPath)
	require.NoError(t, err, "socket must still exist on disk after older session closes")
	require.True(t, os.SameFile(fi2, fiCurrent), "socket on disk must belong to newer session")

	// Verify the socket is still functional by dialing it.
	clientConn, err := net.Dial("unix", socketPath)
	require.NoError(t, err, "dialing socket owned by newer session must succeed")
	_ = clientConn.Close()

	// Session 2 closes: it must unlink the socket because it still owns it.
	err = uln2.Close()
	require.NoError(t, err)

	_, err = os.Lstat(socketPath)
	require.ErrorIs(t, err, fs.ErrNotExist, "socket must be unlinked when owner session closes")
}

func TestUnixForwardListener_NormalCloseUnlinksSocket(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "forward.sock")

	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	fi, err := os.Lstat(socketPath)
	require.NoError(t, err)

	ul, ok := ln.(*net.UnixListener)
	require.True(t, ok)
	ul.SetUnlinkOnClose(false)

	uln := &unixForwardListener{
		Listener: ln,
		path:     socketPath,
		fi:       fi,
	}

	err = uln.Close()
	require.NoError(t, err)

	_, err = os.Lstat(socketPath)
	require.ErrorIs(t, err, fs.ErrNotExist, "socket must be unlinked after normal close")
}

func TestServer_StreamLocalForward_OlderSessionDoesNotUnlinkNewerSessionSocket(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	logger := testutil.Logger(t)
	s, err := NewServer(ctx, logger.Named("ssh-server"), prometheus.NewRegistry(), afero.NewMemMapFs(), agentexec.DefaultExecer, nil)
	require.NoError(t, err)
	defer s.Close()
	err = s.UpdateHostSigner(42)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Serve(ln)
	}()
	defer func() {
		_ = s.Close()
		<-done
	}()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "forward.sock")

	// Client 1 connects and forwards socketPath
	c1 := testSSHClient(t, ln.Addr().String())
	payload1 := gossh.Marshal(&streamLocalForwardPayload{SocketPath: socketPath})
	ok, _, err := c1.SendRequest("streamlocal-forward@openssh.com", true, payload1)
	require.NoError(t, err)
	require.True(t, ok, "first forward request must succeed")

	fi1, err := os.Lstat(socketPath)
	require.NoError(t, err)

	// Client 2 connects and forwards the same socketPath
	c2 := testSSHClient(t, ln.Addr().String())
	payload2 := gossh.Marshal(&streamLocalForwardPayload{SocketPath: socketPath})
	ok, _, err = c2.SendRequest("streamlocal-forward@openssh.com", true, payload2)
	require.NoError(t, err)
	require.True(t, ok, "second forward request must succeed and override socket")

	fi2, err := os.Lstat(socketPath)
	require.NoError(t, err)
	require.False(t, os.SameFile(fi1, fi2), "second forward must create a new socket file")

	// Close Client 1 (older session)
	err = c1.Close()
	require.NoError(t, err)

	// Give a moment for cleanup goroutines to run
	require.Eventually(t, func() bool {
		fi, err := os.Lstat(socketPath)
		return err == nil && os.SameFile(fi2, fi)
	}, testutil.WaitShort, testutil.IntervalFast, "socket must still exist and belong to client 2")

	// Client 2 can receive connections on socketPath
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err, "connecting to overridden socket must succeed")
	_ = conn.Close()

	// Close Client 2 (newer session)
	err = c2.Close()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := os.Lstat(socketPath)
		return errors.Is(err, fs.ErrNotExist)
	}, testutil.WaitShort, testutil.IntervalFast, "socket must be unlinked when owner session closes")
}

func testSSHClient(t *testing.T, addr string) *gossh.Client {
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	sshConn, channels, requests, err := gossh.NewClientConn(conn, "localhost:22", &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // This is a test.
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshConn.Close()
	})
	c := gossh.NewClient(sshConn, channels, requests)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}
