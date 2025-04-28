package agentssh_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
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

	ctx := context.Background()
	logger := testutil.Logger(t)
	fs := afero.NewOsFs()
	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), fs, agentexec.DefaultExecer, &agentssh.Config{})
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
	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", agentssh.X11StartPort+displayNumber))
		if err == nil {
			_, err = conn.Write([]byte(payload))
			assert.NoError(t, err)
			_ = conn.Close()
		}
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast)

	x11 := <-x11Chans
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
