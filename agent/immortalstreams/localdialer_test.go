//nolint:testpackage
package immortalstreams

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// Removed: Tests for determineDialStrategy which no longer exists.

func TestLocalDialer_LocalDial(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	dialer := NewLocalDialer(logger)

	// Start a test server
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in the background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	// Test local dial
	conn, err := dialer.DialContext(ctx, listener.Addr().String())
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())

	// Test with localhost hostname
	conn, err = dialer.DialContext(ctx, net.JoinHostPort("localhost", strconv.Itoa(port)))
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())
}

func TestLocalDialer_UpdateTailnetConn(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	dialer := NewLocalDialer(logger)

	// Initially no tailnet connection
	require.Nil(t, dialer.tailnetConn)
	// agent address is no longer tracked in the dialer

	// Update with a mock tailnet connection (nil is ok for this test)
	dialer.UpdateTailnetConn(nil)

	// agent address is ignored by the dialer; ensure tailnetConn can be updated without panic
	require.Nil(t, dialer.tailnetConn)
}

func TestLocalDialer_DialContext_InvalidAddress(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	dialer := NewLocalDialer(logger)

	// Test invalid address format
	_, err := dialer.DialContext(ctx, "invalid-address")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse address")

	// Test invalid port
	_, err = dialer.DialContext(ctx, "localhost:invalid-port")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse port")
}

func TestLocalDialer_DialContext_ConnectionRefused(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	dialer := NewLocalDialer(logger)

	// Try to connect to a port that's not listening
	_, err := dialer.DialContext(ctx, "localhost:65535")
	require.Error(t, err)
	// The error should be a connection refused error
	require.True(t, isConnectionRefusedError(err), "expected connection refused error, got: %v", err)
}

// Unsupported network test removed; LocalDialer always uses TCP.

// Removed: Tests asserting localhost strategy via determineDialStrategy.
