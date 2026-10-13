package tailnet

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

func TestPeerPathFromStatus(t *testing.T) {
	t.Parallel()

	require.Equal(t, PeerPath{Type: PathUnknown}, peerPathFromStatus(nil))
	require.Equal(t, PeerPath{Type: PathUnknown}, peerPathFromStatus(&ipnstate.PeerStatus{
		CurAddr: "1.2.3.4:5",
		Relay:   "r",
	}), "no handshake")
	require.Equal(t, PeerPath{Type: PathDERP, DERPRegion: "r"}, peerPathFromStatus(&ipnstate.PeerStatus{
		Relay:         "r",
		LastHandshake: time.Now(),
	}))
	require.Equal(t, PeerPath{Type: PathDirect, Endpoint: "1.2.3.4:5", DERPRegion: "r"}, peerPathFromStatus(&ipnstate.PeerStatus{
		CurAddr:       "1.2.3.4:5",
		Relay:         "r",
		LastHandshake: time.Now(),
	}))
}

func TestPathWatchersDispatch(t *testing.T) {
	t.Parallel()

	keyA := key.NewNode().Public()
	keyB := key.NewNode().Public()
	type change struct{ prev, cur PeerPath }
	var gotA, gotB []change

	w := &pathWatchers{subs: make(map[*pathSubscription]struct{})}
	subA := &pathSubscription{
		nodeKey: keyA,
		last:    PeerPath{Type: PathUnknown},
		fn:      func(prev, cur PeerPath) { gotA = append(gotA, change{prev, cur}) },
	}
	subB := &pathSubscription{
		nodeKey: keyB,
		last:    PeerPath{Type: PathUnknown},
		fn:      func(prev, cur PeerPath) { gotB = append(gotB, change{prev, cur}) },
	}
	w.subs[subA] = struct{}{}
	w.subs[subB] = struct{}{}

	status := func(peers map[key.NodePublic]*ipnstate.PeerStatus) *ipnstate.Status {
		return &ipnstate.Status{Peer: peers}
	}
	now := time.Now()
	derp := &ipnstate.PeerStatus{Relay: "r", LastHandshake: now}
	direct1 := &ipnstate.PeerStatus{Relay: "r", CurAddr: "1.1.1.1:1", LastHandshake: now}
	direct2 := &ipnstate.PeerStatus{Relay: "r", CurAddr: "2.2.2.2:2", LastHandshake: now}

	w.dispatch(status(map[key.NodePublic]*ipnstate.PeerStatus{keyA: derp}))
	require.Equal(t, []change{{PeerPath{Type: PathUnknown}, PeerPath{Type: PathDERP, DERPRegion: "r"}}}, gotA)
	require.Empty(t, gotB, "peer B has no status")

	// No change, no callback.
	w.dispatch(status(map[key.NodePublic]*ipnstate.PeerStatus{keyA: derp}))
	require.Len(t, gotA, 1)

	// DERP to direct, then a direct endpoint change.
	w.dispatch(status(map[key.NodePublic]*ipnstate.PeerStatus{keyA: direct1}))
	w.dispatch(status(map[key.NodePublic]*ipnstate.PeerStatus{keyA: direct2}))
	require.Len(t, gotA, 3)
	require.Equal(t, PathDirect, gotA[1].cur.Type)
	require.Equal(t, "2.2.2.2:2", gotA[2].cur.Endpoint)
	require.Equal(t, "1.1.1.1:1", gotA[2].prev.Endpoint)

	// Canceled subscriptions are never notified, even if dispatch already
	// snapshotted them.
	subA.canceled = true
	w.dispatch(status(map[key.NodePublic]*ipnstate.PeerStatus{keyA: derp, keyB: derp}))
	require.Len(t, gotA, 3)
	require.Len(t, gotB, 1)
}

type wrappedConn struct {
	net.Conn
}

func (w *wrappedConn) NetConn() net.Conn { return w.Conn }

func TestPeerConnFromConn(t *testing.T) {
	t.Parallel()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	pc := &PeerConn{Conn: a}
	got, ok := PeerConnFromConn(&wrappedConn{Conn: &wrappedConn{Conn: pc}})
	require.True(t, ok)
	require.Same(t, pc, got)

	_, ok = PeerConnFromConn(b)
	require.False(t, ok)
	_, ok = PeerConnFromConn(&wrappedConn{Conn: b})
	require.False(t, ok)

	unknown := &PeerConn{Conn: a}
	_, known := unknown.PeerID()
	require.False(t, known)
	require.Equal(t, PathUnknown, unknown.Path().Type)
	cur, cancel := unknown.WatchPath(func(_, _ PeerPath) { t.Fatal("unexpected callback") })
	cancel()
	require.Equal(t, PathUnknown, cur.Type)
}
