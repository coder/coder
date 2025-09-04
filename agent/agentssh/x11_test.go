package agentssh_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/testutil"
)

func TestServer_X11(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("X11 forwarding is only supported on Linux")
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := testutil.Logger(t)
	fs := afero.NewMemMapFs()

	// Use in-process networking for X11 forwarding.
	inproc := testutil.NewInProcNet()

	// Create server config with custom X11 listener.
	cfg := &agentssh.Config{
		X11Net: inproc,
	}

	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, cfg)
	require.NoError(t, err)
	defer s.Close()
	err = s.UpdateHostSigner(42)
	assert.NoError(t, err)

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

	wantScreenNumber := 1
	reply, err := sess.SendRequest("x11-req", true, gossh.Marshal(ssh.X11{
		AuthProtocol: "MIT-MAGIC-COOKIE-1",
		AuthCookie:   hex.EncodeToString([]byte("cookie")),
		ScreenNumber: uint32(wantScreenNumber),
	}))
	require.NoError(t, err)
	assert.True(t, reply)

	// Want: ~DISPLAY=localhost:10.1
	out, err := sess.Output("echo DISPLAY=$DISPLAY")
	require.NoError(t, err)

	sc := bufio.NewScanner(bytes.NewReader(out))
	displayNumber := -1
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		t.Log(line)
		if strings.HasPrefix(line, "DISPLAY=") {
			parts := strings.SplitN(line, "=", 2)
			display := parts[1]
			parts = strings.SplitN(display, ":", 2)
			parts = strings.SplitN(parts[1], ".", 2)
			displayNumber, err = strconv.Atoi(parts[0])
			require.NoError(t, err)
			assert.GreaterOrEqual(t, displayNumber, 10, "display number should be >= 10")
			gotScreenNumber, err := strconv.Atoi(parts[1])
			require.NoError(t, err)
			assert.Equal(t, wantScreenNumber, gotScreenNumber, "screen number should match")
			break
		}
	}
	require.NoError(t, sc.Err())
	require.NotEqual(t, -1, displayNumber)

	x11Chans := c.HandleChannelOpen("x11")
	payload := "hello world"
	go func() {
		conn, err := inproc.Dial(ctx, testutil.NewAddr("tcp", fmt.Sprintf("localhost:%d", agentssh.X11StartPort+displayNumber)))
		assert.NoError(t, err)
		_, err = conn.Write([]byte(payload))
		assert.NoError(t, err)
		_ = conn.Close()
	}()

	x11 := testutil.RequireReceive(ctx, t, x11Chans)
	ch, reqs, err := x11.Accept()
	require.NoError(t, err)
	go gossh.DiscardRequests(reqs)
	got := make([]byte, len(payload))
	_, err = ch.Read(got)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got))
	_ = ch.Close()
	_ = s.Close()
	<-done

	// Ensure the Xauthority file was written!
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	_, err = fs.Stat(filepath.Join(home, ".Xauthority"))
	require.NoError(t, err)
}

func TestServer_X11_EvictionLRU(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("X11 forwarding is only supported on Linux")
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := testutil.Logger(t)
	fs := afero.NewMemMapFs()

	// Use in-process networking for X11 forwarding.
	inproc := testutil.NewInProcNet()

	cfg := &agentssh.Config{
		X11Net: inproc,
	}

	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, cfg)
	require.NoError(t, err)
	defer s.Close()
	err = s.UpdateHostSigner(42)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := testutil.Go(t, func() {
		err := s.Serve(ln)
		assert.Error(t, err)
	})

	c := sshClient(t, ln.Addr().String())

	// block off one port to test x11Forwarder evicts at highest port, not number of listeners.
	externalListener, err := inproc.Listen("tcp",
		fmt.Sprintf("localhost:%d", agentssh.X11StartPort+agentssh.X11DefaultDisplayOffset+1))
	require.NoError(t, err)
	defer externalListener.Close()

	// Calculate how many simultaneous X11 sessions we can create given the
	// configured port range.

	startPort := agentssh.X11StartPort + agentssh.X11DefaultDisplayOffset
	maxSessions := agentssh.X11MaxPort - startPort + 1 - 1 // -1 for the blocked port
	require.Greater(t, maxSessions, 0, "expected a positive maxSessions value")

	// shellSession holds references to the session and its standard streams so
	// that the test can keep them open (and optionally interact with them) for
	// the lifetime of the test. If we don't start the Shell with pipes in place,
	// the session will be torn down asynchronously during the test.
	type shellSession struct {
		sess   *gossh.Session
		stdin  io.WriteCloser
		stdout io.Reader
		stderr io.Reader
		// scanner is used to read the output of the session, line by line.
		scanner *bufio.Scanner
	}

	sessions := make([]shellSession, 0, maxSessions)
	for i := 0; i < maxSessions; i++ {
		sess, err := c.NewSession()
		require.NoError(t, err)

		_, err = sess.SendRequest("x11-req", true, gossh.Marshal(ssh.X11{
			AuthProtocol: "MIT-MAGIC-COOKIE-1",
			AuthCookie:   hex.EncodeToString([]byte(fmt.Sprintf("cookie%d", i))),
			ScreenNumber: uint32(0),
		}))
		require.NoError(t, err)

		stdin, err := sess.StdinPipe()
		require.NoError(t, err)
		stdout, err := sess.StdoutPipe()
		require.NoError(t, err)
		stderr, err := sess.StderrPipe()
		require.NoError(t, err)
		require.NoError(t, sess.Shell())

		// The SSH server lazily starts the session. We need to write a command
		// and read back to ensure the X11 forwarding is started.
		scanner := bufio.NewScanner(stdout)
		msg := fmt.Sprintf("ready-%d", i)
		_, err = stdin.Write([]byte("echo " + msg + "\n"))
		require.NoError(t, err)
		// Read until we get the message (first token may be empty due to shell prompt)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, msg) {
				break
			}
		}
		require.NoError(t, scanner.Err())

		sessions = append(sessions, shellSession{
			sess:    sess,
			stdin:   stdin,
			stdout:  stdout,
			stderr:  stderr,
			scanner: scanner,
		})
	}

	// Connect X11 forwarding to the first session. This is used to test that
	// connecting counts as a use of the display.
	x11Chans := c.HandleChannelOpen("x11")
	payload := "hello world"
	go func() {
		conn, err := inproc.Dial(ctx, testutil.NewAddr("tcp", fmt.Sprintf("localhost:%d", agentssh.X11StartPort+agentssh.X11DefaultDisplayOffset)))
		if !assert.NoError(t, err) {
			return
		}
		_, err = conn.Write([]byte(payload))
		assert.NoError(t, err)
		_ = conn.Close()
	}()

	x11 := testutil.RequireReceive(ctx, t, x11Chans)
	ch, reqs, err := x11.Accept()
	require.NoError(t, err)
	go gossh.DiscardRequests(reqs)
	got := make([]byte, len(payload))
	_, err = ch.Read(got)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got))
	_ = ch.Close()

	// Create one more session which should evict a session and reuse the display.
	// The first session was used to connect X11 forwarding, so it should not be evicted.
	// Therefore, the second session should be evicted and its display reused.
	extraSess, err := c.NewSession()
	require.NoError(t, err)

	_, err = extraSess.SendRequest("x11-req", true, gossh.Marshal(ssh.X11{
		AuthProtocol: "MIT-MAGIC-COOKIE-1",
		AuthCookie:   hex.EncodeToString([]byte("extra")),
		ScreenNumber: uint32(0),
	}))
	require.NoError(t, err)

	// Ask the remote side for the DISPLAY value so we can extract the display
	// number that was assigned to this session.
	out, err := extraSess.Output("echo DISPLAY=$DISPLAY")
	require.NoError(t, err)

	// Example output line: "DISPLAY=localhost:10.0".
	var newDisplayNumber int
	{
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "DISPLAY=") {
				parts := strings.SplitN(line, ":", 2)
				require.Len(t, parts, 2)
				displayPart := parts[1]
				if strings.Contains(displayPart, ".") {
					displayPart = strings.SplitN(displayPart, ".", 2)[0]
				}
				var convErr error
				newDisplayNumber, convErr = strconv.Atoi(displayPart)
				require.NoError(t, convErr)
				break
			}
		}
		require.NoError(t, sc.Err())
	}

	// The display number reused should correspond to the SECOND session (display offset 12)
	expectedDisplay := agentssh.X11DefaultDisplayOffset + 2 // +1 was blocked port
	assert.Equal(t, expectedDisplay, newDisplayNumber, "second session should have been evicted and its display reused")

	// First session should still be alive: send cmd and read output.
	msgFirst := "still-alive"
	_, err = sessions[0].stdin.Write([]byte("echo " + msgFirst + "\n"))
	require.NoError(t, err)
	for sessions[0].scanner.Scan() {
		line := strings.TrimSpace(sessions[0].scanner.Text())
		if strings.Contains(line, msgFirst) {
			break
		}
	}
	require.NoError(t, sessions[0].scanner.Err())

	// Second session should now be closed.
	_, err = sessions[1].stdin.Write([]byte("echo dead\n"))
	require.ErrorIs(t, err, io.EOF)
	err = sessions[1].sess.Wait()
	require.Error(t, err)

	// Cleanup.
	for i, sh := range sessions {
		if i == 1 {
			// already closed
			continue
		}
		err = sh.stdin.Close()
		require.NoError(t, err)
		err = sh.sess.Wait()
		require.NoError(t, err)
	}
	err = extraSess.Close()
	require.ErrorIs(t, err, io.EOF)

	err = s.Close()
	require.NoError(t, err)
	_ = testutil.TryReceive(ctx, t, done)
}
