package agent

import (
	"context"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/mem"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestReportConnectionEmpty tests that reportConnection() doesn't choke if given an empty IP string, which is what we
// send if we cannot get the remote address.
func TestReportConnectionEmpty(t *testing.T) {
	t.Parallel()
	connID := uuid.UUID{1}
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := testutil.Context(t, testutil.WaitShort)

	uut := &agent{
		hardCtx: ctx,
		logger:  logger,
	}
	disconnected := uut.reportConnection(connID, proto.Connection_TYPE_UNSPECIFIED, "")

	require.Len(t, uut.reportConnections, 1)
	req0 := uut.reportConnections[0]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req0.GetConnection().GetType())
	require.Equal(t, "", req0.GetConnection().Ip)
	require.Equal(t, connID[:], req0.GetConnection().GetId())
	require.Equal(t, proto.Connection_CONNECT, req0.GetConnection().GetAction())

	disconnected(0, "because")
	require.Len(t, uut.reportConnections, 2)
	req1 := uut.reportConnections[1]
	require.Equal(t, proto.Connection_TYPE_UNSPECIFIED, req1.GetConnection().GetType())
	require.Equal(t, "", req1.GetConnection().Ip)
	require.Equal(t, connID[:], req1.GetConnection().GetId())
	require.Equal(t, proto.Connection_DISCONNECT, req1.GetConnection().GetAction())
	require.Equal(t, "because", req1.GetConnection().GetReason())
}

// TestCollectPeerLatencies_SerializedPings verifies that
// collectPeerLatencies never has more than one Ping in flight at a
// time, preventing goroutine pile-ups on magicsock.Conn.mu that can
// trigger the wgengine watchdog. See #22864.
func TestCollectPeerLatencies_SerializedPings(t *testing.T) {
	t.Parallel()

	const numPeers = 5
	const pingDelay = 10 * time.Millisecond

	fp := &fakePingerNetwork{
		pingDelay: pingDelay,
	}
	// Build N active peers, each with a unique address.
	for i := range numPeers {
		fp.addActivePeer(i)
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	durations, p2p, derp := collectPeerLatencies(ctx, fp)

	// All peers should have been pinged successfully.
	require.Len(t, durations, numPeers)
	require.Equal(t, 0, p2p)
	require.Equal(t, numPeers, derp)

	// The semaphore must have prevented any concurrent pings.
	maxSeen := fp.maxConcurrent.Load()
	assert.EqualValues(t, 1, maxSeen,
		"expected at most 1 concurrent Ping, got %d", maxSeen)
}

// fakePingerNetwork implements peerPinger for testing. It records the
// maximum number of concurrent Ping calls.
type fakePingerNetwork struct {
	peers  map[key.NodePublic]*ipnstate.PeerStatus
	addrs  map[key.NodePublic][]netip.Prefix
	active atomic.Int32
	// maxConcurrent tracks the high-water mark of in-flight pings.
	maxConcurrent atomic.Int32
	pingDelay     time.Duration
}

func (f *fakePingerNetwork) addActivePeer(i int) {
	if f.peers == nil {
		f.peers = make(map[key.NodePublic]*ipnstate.PeerStatus)
		f.addrs = make(map[key.NodePublic][]netip.Prefix)
	}
	// Deterministic key derived from index.
	var raw [32]byte
	raw[0] = byte(i)
	pub := key.NodePublicFromRaw32(mem.B(raw[:]))
	f.peers[pub] = &ipnstate.PeerStatus{Active: true}
	addr := netip.AddrFrom4([4]byte{
		10, 0, 0, byte(1 + i),
	})
	f.addrs[pub] = []netip.Prefix{
		netip.PrefixFrom(addr, 32),
	}
}

func (f *fakePingerNetwork) Status() *ipnstate.Status {
	return &ipnstate.Status{Peer: f.peers}
}

func (f *fakePingerNetwork) NodeAddresses(
	pub key.NodePublic,
) ([]netip.Prefix, bool) {
	a, ok := f.addrs[pub]
	return a, ok
}

func (f *fakePingerNetwork) Ping(
	ctx context.Context, _ netip.Addr,
) (time.Duration, bool, *ipnstate.PingResult, error) {
	cur := f.active.Add(1)
	defer f.active.Add(-1)

	// Record high-water mark via CAS loop.
	for {
		prev := f.maxConcurrent.Load()
		if cur <= prev {
			break
		}
		if f.maxConcurrent.CompareAndSwap(prev, cur) {
			break
		}
	}

	select {
	case <-time.After(f.pingDelay):
	case <-ctx.Done():
		return 0, false, nil, ctx.Err()
	}
	return f.pingDelay, false, &ipnstate.PingResult{}, nil
}
