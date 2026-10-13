package tailnet

import (
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/key"
)

// PathType is the kind of network path traffic takes to a peer.
type PathType string

const (
	// PathUnknown means the peer has no established path, for example
	// because it has not completed a handshake or is no longer known.
	PathUnknown PathType = "unknown"
	// PathDERP means traffic is relayed through a DERP server.
	PathDERP PathType = "derp"
	// PathDirect means traffic flows over a direct UDP path.
	PathDirect PathType = "direct"
)

// peerPathPollInterval is how often peer status is polled for path changes.
const peerPathPollInterval = time.Second

// PeerPath describes the network path to a peer at a point in time.
type PeerPath struct {
	Type PathType
	// Endpoint is the peer's direct UDP endpoint, set when Type is
	// PathDirect.
	Endpoint string
	// DERPRegion is the region code of the peer's home DERP.
	DERPRegion string
}

func peerPathFromStatus(ps *ipnstate.PeerStatus) PeerPath {
	if ps == nil || ps.LastHandshake.IsZero() {
		return PeerPath{Type: PathUnknown}
	}
	if ps.CurAddr != "" {
		return PeerPath{Type: PathDirect, Endpoint: ps.CurAddr, DERPRegion: ps.Relay}
	}
	return PeerPath{Type: PathDERP, DERPRegion: ps.Relay}
}

// changedFrom reports whether p differs from prev in a way worth reporting:
// a different path type, or a different direct endpoint.
func (p PeerPath) changedFrom(prev PeerPath) bool {
	return p.Type != prev.Type || p.Endpoint != prev.Endpoint
}

// PeerConn is a connection accepted from a Listener on a Conn. It records the
// peer that opened the connection, so the network path to that peer can be
// queried without resolving the remote address again.
type PeerConn struct {
	net.Conn
	conn    *Conn
	peerID  uuid.UUID
	nodeKey key.NodePublic
	// known is false if the peer could not be identified when the connection
	// was accepted.
	known bool
}

func (c *Conn) newPeerConn(nc net.Conn, src netip.Addr) *PeerConn {
	pc := &PeerConn{Conn: nc, conn: c}
	pc.peerID, pc.nodeKey, pc.known = c.configMaps.peerByAddr(src)
	return pc
}

// NetConn returns the underlying connection.
func (p *PeerConn) NetConn() net.Conn {
	return p.Conn
}

// CloseWrite shuts down the writing side of the connection, if supported by
// the underlying connection.
func (p *PeerConn) CloseWrite() error {
	if cw, ok := p.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// CloseRead shuts down the reading side of the connection, if supported by
// the underlying connection.
func (p *PeerConn) CloseRead() error {
	if cr, ok := p.Conn.(interface{ CloseRead() error }); ok {
		return cr.CloseRead()
	}
	return nil
}

// PeerID returns the coordinator ID of the peer that opened the connection,
// and false if the peer was not known when the connection was accepted.
func (p *PeerConn) PeerID() (uuid.UUID, bool) {
	return p.peerID, p.known
}

// Path returns the current network path to the peer.
func (p *PeerConn) Path() PeerPath {
	if !p.known {
		return PeerPath{Type: PathUnknown}
	}
	return p.conn.peerPath(p.nodeKey)
}

// WatchPath calls fn whenever the network path to the peer changes, until
// cancel is called. It returns the path at the time of the call. fn is called
// from a shared goroutine and must not block; it is never called after cancel
// returns.
func (p *PeerConn) WatchPath(fn func(prev, cur PeerPath)) (current PeerPath, cancel func()) {
	if !p.known {
		return PeerPath{Type: PathUnknown}, func() {}
	}
	return p.conn.pathWatchers.watch(p.conn, p.nodeKey, fn)
}

// PeerConnFromConn returns the PeerConn underlying nc, unwrapping connections
// that expose their inner connection with a NetConn method.
func PeerConnFromConn(nc net.Conn) (*PeerConn, bool) {
	for nc != nil {
		if pc, ok := nc.(*PeerConn); ok {
			return pc, true
		}
		u, ok := nc.(interface{ NetConn() net.Conn })
		if !ok {
			return nil, false
		}
		nc = u.NetConn()
	}
	return nil, false
}

func (c *Conn) peerPath(nodeKey key.NodePublic) PeerPath {
	if c.isClosed() {
		return PeerPath{Type: PathUnknown}
	}
	return peerPathFromStatus(c.Status().Peer[nodeKey])
}

type pathSubscription struct {
	nodeKey key.NodePublic
	fn      func(prev, cur PeerPath)

	// mu serializes fn calls with cancellation, and guards last and
	// canceled.
	mu       sync.Mutex
	last     PeerPath
	canceled bool
}

// pathWatchers polls peer status on behalf of all path subscriptions of a
// Conn, so the status is built once per interval regardless of how many
// connections are watched.
type pathWatchers struct {
	mu      sync.Mutex
	subs    map[*pathSubscription]struct{}
	running bool
}

func (w *pathWatchers) watch(c *Conn, nodeKey key.NodePublic, fn func(prev, cur PeerPath)) (PeerPath, func()) {
	sub := &pathSubscription{
		nodeKey: nodeKey,
		fn:      fn,
		last:    c.peerPath(nodeKey),
	}
	w.mu.Lock()
	if w.subs == nil {
		w.subs = make(map[*pathSubscription]struct{})
	}
	w.subs[sub] = struct{}{}
	start := !w.running
	w.running = true
	w.mu.Unlock()
	if start {
		go w.run(c)
	}

	return sub.last, func() {
		w.mu.Lock()
		delete(w.subs, sub)
		w.mu.Unlock()
		sub.mu.Lock()
		sub.canceled = true
		sub.mu.Unlock()
	}
}

// run polls until the Conn is closed or no subscriptions remain.
func (w *pathWatchers) run(c *Conn) {
	ticker := time.NewTicker(peerPathPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.watchCtx.Done():
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			return
		case <-ticker.C:
		}
		w.mu.Lock()
		if len(w.subs) == 0 {
			w.running = false
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		w.dispatch(c.Status())
	}
}

// dispatch notifies every subscription whose peer path differs in status
// from the last path it was notified about.
func (w *pathWatchers) dispatch(status *ipnstate.Status) {
	w.mu.Lock()
	subs := make([]*pathSubscription, 0, len(w.subs))
	for sub := range w.subs {
		subs = append(subs, sub)
	}
	w.mu.Unlock()

	for _, sub := range subs {
		cur := peerPathFromStatus(status.Peer[sub.nodeKey])
		sub.mu.Lock()
		if !sub.canceled && cur.changedFrom(sub.last) {
			prev := sub.last
			sub.last = cur
			sub.fn(prev, cur)
		}
		sub.mu.Unlock()
	}
}
