package drpcsdk_test

import (
	"net"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

// TestYamuxConfig_ClosedSessionIsCollected checks that a closed yamux session
// becomes unreachable once the session is closed, even when a stream was
// closed locally before the peer's FIN arrived. With yamux's default
// StreamCloseTimeout, the timer armed for such a stream keeps the session
// reachable for five minutes after it closes.
func TestYamuxConfig_ClosedSessionIsCollected(t *testing.T) {
	t.Parallel()

	ptr := openAndCloseSession(t)
	require.Eventually(t, func() bool {
		runtime.GC()
		return ptr.Value() == nil
	}, testutil.WaitShort, testutil.IntervalFast)
}

func openAndCloseSession(t *testing.T) weak.Pointer[yamux.Session] {
	t.Helper()

	clientConn, serverConn := net.Pipe()
	client, err := yamux.Client(clientConn, drpcsdk.YamuxConfig())
	require.NoError(t, err)
	server, err := yamux.Server(serverConn, drpcsdk.YamuxConfig())
	require.NoError(t, err)

	accepted := make(chan net.Conn, 1)
	go func() {
		stream, err := server.AcceptStream()
		if err != nil {
			return
		}
		accepted <- stream
	}()

	stream, err := client.OpenStream()
	require.NoError(t, err)
	select {
	case <-accepted:
	case <-time.After(testutil.WaitShort):
		t.Fatal("server did not accept the stream")
	}

	// Close the client side only, so the stream is waiting on the server's
	// FIN when the sessions shut down.
	require.NoError(t, stream.Close())
	require.NoError(t, client.Close())
	require.NoError(t, server.Close())

	return weak.Make(client)
}
