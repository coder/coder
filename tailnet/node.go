package tailnet

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
	"tailscale.com/wgengine"

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
	lastStatus           time.Time
	blockEndpoints       bool
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
			u.logger.Debug(context.Background(), "closing nodeUpdater updateLoop")
			return
		}
		u.dirty = false
		u.phase = configuring
		u.Broadcast()

		callback := u.callback
		if callback == nil {
			u.logger.Debug(context.Background(), "skipped sending node; no node callback")
			continue
		}

		// We cannot reach nodes without DERP for discovery. Therefore, there is no point in sending
		// the node without this, and we can save ourselves from churn in the tailscale/wireguard
		// layer.
		node := u.nodeLocked()
		if node.PreferredDERP == 0 {
			u.logger.Debug(context.Background(), "skipped sending node; no PreferredDERP", slog.F("node", node))
			continue
		}

		u.L.Unlock()
		u.logger.Debug(context.Background(), "calling nodeUpdater callback", slog.F("node", node))
		callback(node)
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
		phased:               phased{Cond: *(sync.NewCond(&sync.Mutex{}))},
		logger:               logger,
		id:                   id,
		key:                  np,
		discoKey:             dp,
		derpForcedWebsockets: make(map[int]string),
		callback:             callback,
	}
	go u.updateLoop()
	return u
}

// nodeLocked returns the current best node information.  u.L must be held.
func (u *nodeUpdater) nodeLocked() *Node {
	var endpoints []string
	if !u.blockEndpoints {
		endpoints = slices.Clone(u.endpoints)
	}
	return &Node{
		ID:                  u.id,
		AsOf:                dbtime.Now(),
		Key:                 u.key,
		Addresses:           slices.Clone(u.addresses),
		AllowedIPs:          slices.Clone(u.addresses),
		DiscoKey:            u.discoKey,
		Endpoints:           endpoints,
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
		u.logger.Debug(context.Background(), "new preferred DERP",
			slog.F("preferred_derp", u.preferredDERP))
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

// setDERPForcedWebsocket handles callbacks from the magicConn about DERP regions that are forced to
// use websockets (instead of Upgrade: derp).  This information is for debugging only.
func (u *nodeUpdater) setDERPForcedWebsocket(region int, reason string) {
	u.L.Lock()
	defer u.L.Unlock()
	dirty := u.derpForcedWebsockets[region] != reason
	u.derpForcedWebsockets[region] = reason
	if dirty {
		u.dirty = true
		u.Broadcast()
	}
}

// setStatus handles the status callback from the wireguard engine to learn about new endpoints
// (e.g. discovered by STUN).  u.L MUST NOT be held
func (u *nodeUpdater) setStatus(s *wgengine.Status, err error) {
	u.logger.Debug(context.Background(), "wireguard status", slog.F("status", s), slog.Error(err))
	if err != nil {
		return
	}
	u.L.Lock()
	defer u.L.Unlock()
	if s.AsOf.Before(u.lastStatus) {
		// Don't process outdated status!
		return
	}
	u.lastStatus = s.AsOf
	endpoints := make([]string, len(s.LocalAddrs))
	for i, ep := range s.LocalAddrs {
		endpoints[i] = ep.Addr.String()
	}
	if slices.Equal(endpoints, u.endpoints) {
		// No need to update the node if nothing changed!
		return
	}
	u.endpoints = endpoints
	u.dirty = true
	u.Broadcast()
}

// setAddresses sets the local addresses for the node. u.L MUST NOT be held.
func (u *nodeUpdater) setAddresses(ips []netip.Prefix) {
	u.L.Lock()
	defer u.L.Unlock()
	if d := prefixesDifferent(u.addresses, ips); !d {
		return
	}
	u.addresses = make([]netip.Prefix, len(ips))
	copy(u.addresses, ips)
	u.dirty = true
	u.Broadcast()
}

// setCallback sets the callback for node changes. It also triggers a call
// for the current node immediately. u.L MUST NOT be held.
func (u *nodeUpdater) setCallback(callback func(node *Node)) {
	u.L.Lock()
	defer u.L.Unlock()
	u.callback = callback
	u.dirty = true
	u.Broadcast()
}

// setBlockEndpoints sets whether we block reporting Node endpoints. u.L MUST NOT
// be held.
// nolint: revive
func (u *nodeUpdater) setBlockEndpoints(blockEndpoints bool) {
	u.L.Lock()
	defer u.L.Unlock()
	if u.blockEndpoints == blockEndpoints {
		return
	}
	u.dirty = true
	u.blockEndpoints = blockEndpoints
	u.Broadcast()
}
