package nats

import (
	"sync"

	"github.com/coder/coder/v2/coderd/database/pubsub"
)

// connTracker tracks the connected state across the NATS Pubsub's owned
// connections. The shared pubsub.Metrics exposes a single connected gauge
// and a disconnect counter; this type holds the NATS-specific quorum logic
// (the gauge is 1 only while every owned connection is up) and drives the
// shared instruments accordingly.
type connTracker struct {
	m *pubsub.BackendMetrics

	// mu guards the connection-state accounting. Connect and disconnect
	// callbacks are rare, so a mutex keeps the gauge update atomic with the
	// count without meaningful contention.
	mu             sync.Mutex
	totalConns     int
	connectedConns int
}

func newConnTracker(m *pubsub.BackendMetrics) *connTracker {
	return &connTracker{m: m}
}

// markConnected records that all total owned connections have dialed
// successfully. The connected gauge is 1 only while every owned
// connection is up.
func (c *connTracker) markConnected(total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalConns = total
	c.connectedConns = total
	c.setConnectedLocked()
}

// markClosed records that the Pubsub is shutting down and forces the
// connected gauge to 0. Closing our own connections does not fire the
// disconnect handler (see NoCallbacksAfterClientClose), so without this
// the gauge would still read 1 after Close. Both counters are zeroed so
// a late reconnect callback during the shutdown window cannot increment
// connectedConns back up and flip the gauge to 1.
func (c *connTracker) markClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.totalConns = 0
	c.connectedConns = 0
	c.m.MarkDisconnected()
}

// onDisconnect records an unexpected disconnect of one owned connection.
func (c *connTracker) onDisconnect() {
	c.m.RecordDisconnect()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connectedConns > 0 {
		c.connectedConns--
	}
	c.setConnectedLocked()
}

// onReconnect records that one owned connection reconnected.
func (c *connTracker) onReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connectedConns < c.totalConns {
		c.connectedConns++
	}
	c.setConnectedLocked()
}

// setConnectedLocked sets the connected gauge to 1 only when every owned
// connection is up. Callers must hold mu.
func (c *connTracker) setConnectedLocked() {
	if c.totalConns > 0 && c.connectedConns == c.totalConns {
		c.m.MarkConnected()
		return
	}
	c.m.MarkDisconnected()
}
