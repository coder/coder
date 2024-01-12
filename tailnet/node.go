package tailnet

import (
	"context"
	"net/netip"
	"sync"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

type nodeUpdater struct {
	phased
	dirty   bool
	closing bool

	// static
	logger   slog.Logger
	id       tailcfg.NodeID
	key      key.NodePublic
	discoKey key.DiscoPublic
	callback func(n *Node)

	// dynamic
	preferredDERP        int
	derpLatency          map[string]float64
	derpForcedWebsockets map[int]string
	endpoints            []string
	addresses            []netip.Prefix
}

// updateLoop waits until the config is dirty and then calls the callback with the newest node.
// It is intended only to be called internally, and shuts down when close() is called.
func (u *nodeUpdater) updateLoop() {
	u.L.Lock()
	defer u.L.Unlock()
	defer func() {
		u.phase = closed
		u.Broadcast()
	}()
	for {
		for !(u.closing || u.dirty) {
			u.phase = idle
			u.Wait()
		}
		if u.closing {
			return
		}
		node := u.nodeLocked()
		u.dirty = false
		u.phase = configuring
		u.Broadcast()

		// We cannot reach nodes without DERP for discovery. Therefore, there is no point in sending
		// the node without this, and we can save ourselves from churn in the tailscale/wireguard
		// layer.
		if node.PreferredDERP == 0 {
			u.logger.Debug(context.Background(), "skipped sending node; no PreferredDERP", slog.F("node", node))
			continue
		}

		u.L.Unlock()
		u.callback(node)
		u.L.Lock()
	}
}

// close closes the nodeUpdate and stops it calling the node callback
func (u *nodeUpdater) close() {
	u.L.Lock()
	defer u.L.Unlock()
	u.closing = true
	u.Broadcast()
	for u.phase != closed {
		u.Wait()
	}
}

func newNodeUpdater(
	logger slog.Logger, callback func(n *Node),
	id tailcfg.NodeID, np key.NodePublic, dp key.DiscoPublic,
) *nodeUpdater {
	u := &nodeUpdater{
		phased:   phased{Cond: *(sync.NewCond(&sync.Mutex{}))},
		logger:   logger,
		id:       id,
		key:      np,
		discoKey: dp,
		callback: callback,
	}
	go u.updateLoop()
	return u
}

// nodeLocked returns the current best node information.  u.L must be held.
func (u *nodeUpdater) nodeLocked() *Node {
	return &Node{
		ID:                  u.id,
		AsOf:                dbtime.Now(),
		Key:                 u.key,
		Addresses:           slices.Clone(u.addresses),
		AllowedIPs:          slices.Clone(u.addresses),
		DiscoKey:            u.discoKey,
		Endpoints:           slices.Clone(u.endpoints),
		PreferredDERP:       u.preferredDERP,
		DERPLatency:         maps.Clone(u.derpLatency),
		DERPForcedWebsocket: maps.Clone(u.derpForcedWebsockets),
	}
}

// setNetInfo processes a NetInfo update from the wireguard engine.  c.L MUST
// NOT be held.
func (u *nodeUpdater) setNetInfo(ni *tailcfg.NetInfo) {
	u.L.Lock()
	defer u.L.Unlock()
	dirty := false
	if u.preferredDERP != ni.PreferredDERP {
		dirty = true
		u.preferredDERP = ni.PreferredDERP
	}
	if !maps.Equal(u.derpLatency, ni.DERPLatency) {
		dirty = true
		u.derpLatency = ni.DERPLatency
	}
	if dirty {
		u.dirty = true
		u.Broadcast()
	}
}
