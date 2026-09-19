package agentegress

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestExitNodeSelectorKeepCurrent(t *testing.T) {
	t.Parallel()

	first := netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1")
	current := netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1")
	added := netip.MustParseAddrPort("[fd7a:115c:a1e0::3]:1")
	previous := newExitNodeSelector(slogtest.Make(t, nil), quartz.NewMock(t), time.Second, []netip.AddrPort{first, current})
	previous.current = 1
	next := newExitNodeSelector(slogtest.Make(t, nil), quartz.NewMock(t), time.Second, []netip.AddrPort{added, current, first})

	next.keepCurrent(previous)
	require.Equal(t, 1, next.current)
}

func TestExitNodeSelectorFailoverAndRecovery(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	preferred := netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1")
	fallback := netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1")
	selector := newExitNodeSelector(slogtest.Make(t, nil), clock, time.Second, []netip.AddrPort{preferred, fallback})

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

func TestExitNodeSelectorDialTimeout(t *testing.T) {
	t.Parallel()

	preferred := netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1")
	fallback := netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1")
	selector := newExitNodeSelector(slogtest.Make(t, nil), quartz.NewMock(t), testutil.IntervalFast, []netip.AddrPort{preferred, fallback})

	var dialed []netip.AddrPort
	dialer := DialerFunc(func(ctx context.Context, addr netip.AddrPort) (net.Conn, error) {
		dialed = append(dialed, addr)
		if addr == preferred {
			<-ctx.Done()
			return nil, ctx.Err()
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
}

func TestExitNodeSelectorAllFail(t *testing.T) {
	t.Parallel()

	selector := newExitNodeSelector(slogtest.Make(t, nil), quartz.NewMock(t), time.Second, []netip.AddrPort{
		netip.MustParseAddrPort("[fd7a:115c:a1e0::1]:1"),
		netip.MustParseAddrPort("[fd7a:115c:a1e0::2]:1"),
	})
	_, _, err := selector.dial(t.Context(), DialerFunc(func(context.Context, netip.AddrPort) (net.Conn, error) {
		return nil, xerrors.New("unreachable")
	}))
	require.ErrorContains(t, err, "dial exit nodes")
}
