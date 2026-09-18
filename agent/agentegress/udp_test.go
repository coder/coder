package agentegress_test

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fixedOrigDst makes every redirected datagram look like it was sent to
// dst, standing in for the netfilter control message.
func fixedOrigDst(dst netip.AddrPort) func([]byte) netip.AddrPort {
	return func([]byte) netip.AddrPort { return dst }
}

func deadlineFrom(ctx context.Context) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Now().Add(testutil.WaitShort)
	}
	return deadline
}

// udpClient returns a connected UDP socket to the proxy's UDP listener.
func udpClient(t testing.TB, addr netip.AddrPort) *net.UDPConn {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(addr))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestUDP_RelaysToFakeIPByName(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	// The fake IP is only known once the proxy exists, so the hook reads it
	// through an atomic.
	var dst atomic.Pointer[netip.AddrPort]
	proxy := startProxy(t, exit, proxyOptions{
		udpOrigDst: func([]byte) netip.AddrPort {
			if d := dst.Load(); d != nil {
				return *d
			}
			return netip.AddrPort{}
		},
	})
	fakeDst := netip.AddrPortFrom(proxy.FakeIPForTest("quic.example.com"), 4433)
	dst.Store(&fakeDst)
	ctx := testutil.Context(t, testutil.WaitShort)

	client := udpClient(t, proxy.UDPAddr())
	deadline, _ := ctx.Deadline()
	require.NoError(t, client.SetDeadline(deadline))

	// Several datagrams on one flow share a single stream, including ones
	// sent before the CONNECT completes.
	for _, msg := range []string{"one", "two", "three"} {
		_, err := client.Write([]byte(msg))
		require.NoError(t, err)
	}
	buf := make([]byte, 1500)
	got := map[string]bool{}
	for range 3 {
		n, err := client.Read(buf)
		require.NoError(t, err)
		got[string(buf[:n])] = true
	}
	require.Equal(t, map[string]bool{"echo:one": true, "echo:two": true, "echo:three": true}, got)

	connects := exit.connectsTo("quic.example.com:4433")
	require.Len(t, connects, 1)
	require.Equal(t, "udp", connects[0].proto)
	require.Len(t, exit.udpFramesFor("quic.example.com:4433"), 3)
}

func TestUDP_RelaysToIPLiteral(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		udpOrigDst: fixedOrigDst(netip.MustParseAddrPort("192.0.2.7:123")),
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	client := udpClient(t, proxy.UDPAddr())
	deadline, _ := ctx.Deadline()
	require.NoError(t, client.SetDeadline(deadline))
	_, err := client.Write([]byte("ntp"))
	require.NoError(t, err)
	buf := make([]byte, 1500)
	n, err := client.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "echo:ntp", string(buf[:n]))
	require.Len(t, exit.connectsTo("192.0.2.7:123"), 1)
}

func TestUDP_DeniedFlowIsNegativeCached(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, func(target string) (string, bool) {
		return "udp blocked", target == "blocked.example.com:5000"
	})
	mClock := quartz.NewMock(t)
	// The hook runs once per datagram in arrival order. Datagrams from one
	// loopback socket arrive in send order, so the sentinel positions (the
	// 7th and 13th datagrams) are steered to an allowed IP literal and
	// everything else to the denied flow.
	var (
		blocked atomic.Pointer[netip.AddrPort]
		calls   atomic.Int32
	)
	allowed := netip.MustParseAddrPort("192.0.2.8:5000")
	proxy := startProxy(t, exit, proxyOptions{
		clock: mClock,
		udpOrigDst: func([]byte) netip.AddrPort {
			switch calls.Add(1) {
			case 7, 13:
				return allowed
			}
			return *blocked.Load()
		},
	})
	blockedDst := netip.AddrPortFrom(proxy.FakeIPForTest("blocked.example.com"), 5000)
	blocked.Store(&blockedDst)
	ctx := testutil.Context(t, testutil.WaitShort)

	client := udpClient(t, proxy.UDPAddr())
	require.NoError(t, client.SetDeadline(deadlineFrom(ctx)))
	_, err := client.Write([]byte("first"))
	require.NoError(t, err)
	// The first datagram triggers exactly one CONNECT, which is denied.
	require.Eventually(t, func() bool {
		return len(exit.connectsTo("blocked.example.com:5000")) == 1
	}, testutil.WaitShort, testutil.IntervalFast)

	// Further datagrams inside the negative cache window never reach the
	// exit node. The read loop is sequential, so once the sentinel datagram
	// to the allowed destination has been echoed, the denied ones were
	// already processed.
	sendUntilSentinel := func() {
		t.Helper()
		for range 5 {
			_, err := client.Write([]byte("again"))
			require.NoError(t, err)
		}
		_, err = client.Write([]byte("sentinel"))
		require.NoError(t, err)
		buf := make([]byte, 64)
		n, err := client.Read(buf)
		require.NoError(t, err)
		require.Equal(t, "echo:sentinel", string(buf[:n]))
	}
	sendUntilSentinel()
	require.Len(t, exit.connectsTo("blocked.example.com:5000"), 1)

	// After the cache entry expires the exit node is asked again.
	mClock.Advance(31 * time.Second).MustWait(ctx)
	sendUntilSentinel()
	require.Eventually(t, func() bool {
		return len(exit.connectsTo("blocked.example.com:5000")) == 2
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestUDP_IdleSessionCloses(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	mClock := quartz.NewMock(t)
	proxy := startProxy(t, exit, proxyOptions{
		clock:      mClock,
		udpOrigDst: fixedOrigDst(netip.MustParseAddrPort("192.0.2.9:9000")),
	})
	ctx := testutil.Context(t, testutil.WaitShort)

	client := udpClient(t, proxy.UDPAddr())
	require.NoError(t, client.SetDeadline(deadlineFrom(ctx)))
	_, err := client.Write([]byte("ping"))
	require.NoError(t, err)
	buf := make([]byte, 64)
	n, err := client.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "echo:ping", string(buf[:n]))
	require.Len(t, exit.connectsTo("192.0.2.9:9000"), 1)

	// The idle timer was armed when the session was created and reset by
	// the reply; advancing to it closes the stream, so the next datagram
	// opens a new one.
	mClock.Advance(60 * time.Second).MustWait(ctx)
	require.Eventually(t, func() bool {
		if _, err := client.Write([]byte("ping2")); err != nil {
			return false
		}
		return len(exit.connectsTo("192.0.2.9:9000")) == 2
	}, testutil.WaitShort, testutil.IntervalMedium)
}

func TestUDP_DropsOversizedAndUnknownDestination(t *testing.T) {
	t.Parallel()

	exit := newFakeExitNode(t, nil)
	proxy := startProxy(t, exit, proxyOptions{
		udpOrigDst: fixedOrigDst(netip.MustParseAddrPort("192.0.2.9:9000")),
	})
	client := udpClient(t, proxy.UDPAddr())
	_, err := client.Write(make([]byte, 1401))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		oversized, _ := proxy.UDPDropCountsForTest()
		return oversized == 1
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Empty(t, exit.connectsTo("192.0.2.9:9000"))

	// Without a recoverable destination nothing is relayed.
	unknown := startProxy(t, exit, proxyOptions{
		udpOrigDst: func([]byte) netip.AddrPort { return netip.AddrPort{} },
	})
	client2 := udpClient(t, unknown.UDPAddr())
	_, err = client2.Write([]byte("lost"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, unknownDst := unknown.UDPDropCountsForTest()
		return unknownDst == 1
	}, testutil.WaitShort, testutil.IntervalFast)
}
