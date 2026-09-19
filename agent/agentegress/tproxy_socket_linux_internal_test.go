//go:build linux

package agentegress

import (
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestBypassControl(t *testing.T) {
	t.Parallel()
	if unix.Geteuid() != 0 {
		t.Skip("setting SO_MARK requires a network capability")
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	var gotMark int
	dialer := net.Dialer{Control: func(network, address string, raw syscall.RawConn) error {
		if err := bypassControl(network, address, raw); err != nil {
			return err
		}
		var optErr error
		if err := raw.Control(func(fd uintptr) {
			gotMark, optErr = unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MARK)
		}); err != nil {
			return err
		}
		return optErr
	}}
	client, err := dialer.DialContext(t.Context(), "tcp4", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.Equal(t, tproxyBypassMarkValue, gotMark)

	select {
	case conn := <-accepted:
		t.Cleanup(func() { _ = conn.Close() })
	case err := <-acceptErr:
		require.NoError(t, err)
	case <-t.Context().Done():
		require.Fail(t, "listener did not accept marked connection")
	}
}
