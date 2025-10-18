//nolint:testpackage
package immortalstreams

import (
	"math"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestLocalDialer_LocalDial(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slogtest.Make(t, nil)
	dialer := NewLocalDialer(logger, nil)

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

	// Ensure port is within uint16 range before casting (satisfies gosec G115)
	if port < 0 || port > int(math.MaxUint16) {
		t.Fatalf("listener port out of range: %d", port)
	}

	// Test local dial
	conn, err := dialer.DialPort(ctx, uint16(port)) //nolint:gosec
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())
}
