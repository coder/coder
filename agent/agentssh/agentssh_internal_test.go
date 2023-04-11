package agentssh

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/codersdk/agentsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

const countingScript = `
i=0
while [ $i -ne 20000 ]
do
        i=$(($i+1))
        echo "$i"
done
`

func TestServer_sessionStart_longoutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger := slogtest.Make(t, nil)
	s, err := NewServer(ctx, logger, 0)
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

	c := SSHTestClient(t, ln.Addr().String())
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

	select {
	case <-readDone:
		// OK
	case <-ctx.Done():
		t.Fatal("read timeout")
	}

	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		err := sess.Wait()
		assert.NoError(t, err)
	}()

	select {
	case <-sessionDone:
		// OK!
	case <-ctx.Done():
		t.Fatal("session timeout")
	}
}

const longScript = `
echo "started"
sleep 30
echo "done"
`

func TestServer_sessionStart_orphan(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger := slogtest.Make(t, nil)
	s, err := NewServer(ctx, logger, 0)
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

	c := SSHTestClient(t, ln.Addr().String())
	sess, err := c.NewSession()
	require.NoError(t, err)

	stdout, err := sess.StdoutPipe()
	require.NoError(t, err)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		s := bufio.NewScanner(stdout)
		require.True(t, s.Scan())
		txt := s.Text()
		assert.Equal(t, "started", txt, "output corrupted")
	}()

	err = sess.Start(longScript)
	require.NoError(t, err)

	select {
	case <-readDone:
		// OK
	case <-ctx.Done():
		t.Fatal("read timeout")
	}

	// process is started, and should be sleeping for ~30 seconds
	// close the session
	err = sess.Close()
	require.NoError(t, err)

	// now, we wait for the handler to complete.  If it does so before the
	// main test timeout, we consider this a pass.  If not, it indicates
	// that the server isn't properly shutting down sessions when they are
	// disconnected client side, which could lead to processes hanging around
	// indefinitely.
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		for {
			select {
			case <-time.After(time.Millisecond * 10):
				s.mu.Lock()
				n := len(s.sessions)
				s.mu.Unlock()
				if n == 0 {
					return
				}
			}
		}
	}()

	select {
	case <-handlerDone:
		// OK!
	case <-ctx.Done():
		t.Fatal("handler timeout")
	}
}

// SSHTestClient creates an ssh.Client for testing
func SSHTestClient(t *testing.T, addr string) *ssh.Client {
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
