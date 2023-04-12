// Package agentssh_test provides tests for basic functinoality of the agentssh
// package, more test coverage can be found in the `agent` and `cli` package(s).
package agentssh_test

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"

	"github.com/coder/coder/agent/agentssh"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNewServer_ServeClient(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogtest.Make(t, nil)
	s, err := agentssh.NewServer(ctx, logger, 0)
	require.NoError(t, err)

	// The assumption is that these are set before serving SSH connections.
	s.AgentToken = func() string { return "" }
	s.Manifest = atomic.NewPointer(&agentsdk.Manifest{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := s.Serve(ln)
		assert.Error(t, err) // Server is closed.
	}()

	c := sshClient(t, ln.Addr().String())
	var b bytes.Buffer
	sess, err := c.NewSession()
	sess.Stdout = &b
	require.NoError(t, err)
	err = sess.Start("echo hello")
	require.NoError(t, err)

	err = sess.Wait()
	require.NoError(t, err)

	require.Equal(t, "hello", strings.TrimSpace(b.String()))

	err = s.Close()
	require.NoError(t, err)
	<-done
}

func TestNewServer_CloseActiveConnections(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	s, err := agentssh.NewServer(ctx, logger, 0)
	require.NoError(t, err)

	// The assumption is that these are set before serving SSH connections.
	s.AgentToken = func() string { return "" }
	s.Manifest = atomic.NewPointer(&agentsdk.Manifest{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := s.Serve(ln)
		assert.Error(t, err) // Server is closed.
	}()

	pty := ptytest.New(t)
	defer pty.Close()

	doClose := make(chan struct{})
	go func() {
		defer wg.Done()
		c := sshClient(t, ln.Addr().String())
		sess, err := c.NewSession()
		sess.Stdin = pty.Input()
		sess.Stdout = pty.Output()
		sess.Stderr = pty.Output()

		assert.NoError(t, err)
		err = sess.Start("")
		assert.NoError(t, err)

		close(doClose)
		err = sess.Wait()
		assert.Error(t, err)
	}()

	<-doClose
	err = s.Close()
	require.NoError(t, err)

	wg.Wait()
}

const countingScript = `
i=0
while [ $i -ne 20000 ]
do
        i=$(($i+1))
        echo "$i"
done
`

// TestServer_sessionStart_longoutput is designed to test running a command that
// produces a lot of output and ensure we don't truncate the output returned
// over SSH.
func TestServer_sessionStart_longoutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger := slogtest.Make(t, nil)
	s, err := agentssh.NewServer(ctx, logger, 0)
	require.NoError(t, err)

	// The assumption is that these are set before serving SSH connections.
	s.AgentToken = func() string { return "" }
	s.Manifest = atomic.NewPointer(&agentsdk.Manifest{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := s.Serve(ln)
		assert.Error(t, err) // Server is closed.
	}()

	c := sshClient(t, ln.Addr().String())
	sess, err := c.NewSession()
	require.NoError(t, err)

	stdout, err := sess.StdoutPipe()
	require.NoError(t, err)
	readDone := make(chan struct{})
	go func() {
		w := 0
		defer close(readDone)
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			w++
			ns := s.Text()
			n, err := strconv.Atoi(ns)
			require.NoError(t, err)
			require.Equal(t, w, n, "output corrupted")
		}
		assert.Equal(t, w, 20000, "output truncated")
		assert.NoError(t, s.Err())
	}()

	err = sess.Start(countingScript)
	require.NoError(t, err)

	waitForChan(t, readDone, ctx, "read timeout")

	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		err := sess.Wait()
		assert.NoError(t, err)
	}()

	waitForChan(t, sessionDone, ctx, "session timeout")
	err = s.Close()
	require.NoError(t, err)
	waitForChan(t, done, ctx, "timeout closing server")
}

func waitForChan(t *testing.T, c <-chan struct{}, ctx context.Context, msg string) {
	t.Helper()
	select {
	case <-c:
		// OK!
	case <-ctx.Done():
		t.Fatal(msg)
	}
}

// sshClient creates an ssh.Client for testing
func sshClient(t *testing.T, addr string) *ssh.Client {
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	sshConn, channels, requests, err := ssh.NewClientConn(conn, "localhost:22", &ssh.ClientConfig{
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // This is a test.
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sshConn.Close()
	})
	c := ssh.NewClient(sshConn, channels, requests)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}
