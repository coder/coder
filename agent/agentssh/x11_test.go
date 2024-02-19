package agentssh_test

import (
	"context"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/testutil"
)

func TestServer_X11(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("X11 forwarding is only supported on Linux")
	}

	ctx := context.Background()
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	fs := afero.NewOsFs()
	dir := t.TempDir()
	s, err := agentssh.NewServer(ctx, logger, prometheus.NewRegistry(), fs, &agentssh.Config{
		X11SocketDir: dir,
	})
	require.NoError(t, err)
	defer s.Close()

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

	reply, err := sess.SendRequest("x11-req", true, gossh.Marshal(ssh.X11{
		AuthProtocol: "MIT-MAGIC-COOKIE-1",
		AuthCookie:   hex.EncodeToString([]byte("cookie")),
		ScreenNumber: 0,
	}))
	require.NoError(t, err)
	assert.True(t, reply)

	err = sess.Shell()
	require.NoError(t, err)

	x11Chans := c.HandleChannelOpen("x11")
	payload := "hello world"
	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", filepath.Join(dir, "X0"))
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
