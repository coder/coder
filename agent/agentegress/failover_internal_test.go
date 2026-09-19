package agentegress

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/quartz"
)

func TestExitNodeSelectorFailoverAndRecovery(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	preferred := netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1")
	fallback := netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1")
	selector := newExitNodeSelector(slogtest.Make(t, nil), clock, []netip.AddrPort{preferred, fallback})

	preferredUp := false
	var dialed []netip.AddrPort
	dialer := DialerFunc(func(_ context.Context, addr netip.AddrPort) (net.Conn, error) {
		dialed = append(dialed, addr)
		if addr == preferred && !preferredUp {
			return nil, xerrors.New("unreachable")
		}
		client, server := net.Pipe()
		t.Cleanup(func() { _ = server.Close() })
		return client, nil
	})

	conn, addr, err := selector.dial(t.Context(), dialer)
	require.NoError(t, err)
	require.Equal(t, fallback, addr)
	require.Equal(t, []netip.AddrPort{preferred, fallback}, dialed)
	require.NoError(t, conn.Close())

	dialed = nil
	conn, addr, err = selector.dial(t.Context(), dialer)
	require.NoError(t, err)
	require.Equal(t, fallback, addr)
	require.Equal(t, []netip.AddrPort{fallback}, dialed)
	require.NoError(t, conn.Close())

	preferredUp = true
	clock.Advance(exitNodeRetryBackoff).MustWait(t.Context())
	dialed = nil
	conn, addr, err = selector.dial(t.Context(), dialer)
	require.NoError(t, err)
	require.Equal(t, preferred, addr)
	require.Equal(t, []netip.AddrPort{preferred}, dialed)
	require.NoError(t, conn.Close())
}

func TestExitNodeSelectorAllFail(t *testing.T) {
	t.Parallel()

	selector := newExitNodeSelector(slogtest.Make(t, nil), quartz.NewMock(t), []netip.AddrPort{
		netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1"),
		netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1"),
	})
	_, _, err := selector.dial(t.Context(), DialerFunc(func(context.Context, netip.AddrPort) (net.Conn, error) {
		return nil, xerrors.New("unreachable")
	}))
	require.ErrorContains(t, err, "dial exit nodes")
}
