package coderd

import (
	"sync"
	"time"

	"github.com/google/uuid"

	tailnetproto "github.com/coder/coder/v2/tailnet/proto"
)

const peerNetworkTelemetryMaxAge = 2 * time.Minute

type PeerNetworkTelemetry struct {
	P2P           *bool
	DERPLatency   *time.Duration
	P2PLatency    *time.Duration
	HomeDERP      int
	LastUpdatedAt time.Time
}

type PeerNetworkTelemetryStore struct {
	mu      sync.RWMutex
	byAgent map[uuid.UUID]map[uuid.UUID]*PeerNetworkTelemetry
}

func NewPeerNetworkTelemetryStore() *PeerNetworkTelemetryStore {
	return &PeerNetworkTelemetryStore{
		byAgent: make(map[uuid.UUID]map[uuid.UUID]*PeerNetworkTelemetry),
	}
}

func (s *PeerNetworkTelemetryStore) Update(agentID, peerID uuid.UUID, event *tailnetproto.TelemetryEvent) {
	if event == nil {
		return
	}
	if event.Status == tailnetproto.TelemetryEvent_DISCONNECTED {
		s.Delete(agentID, peerID)
		return
	}
	if event.Status != tailnetproto.TelemetryEvent_CONNECTED {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	byPeer := s.byAgent[agentID]
	if byPeer == nil {
		byPeer = make(map[uuid.UUID]*PeerNetworkTelemetry)
		s.byAgent[agentID] = byPeer
	}

	existing := byPeer[peerID]
	entry := &PeerNetworkTelemetry{
		LastUpdatedAt: time.Now(),
	}

	// HomeDERP: prefer explicit non-zero value from the event,
	// otherwise preserve the prior known value.
	if event.HomeDerp != 0 {
		entry.HomeDERP = int(event.HomeDerp)
	} else if existing != nil {
		entry.HomeDERP = existing.HomeDERP
	}

	// Determine whether this event carries any mode/latency signal.
	hasNetworkInfo := event.P2PEndpoint != nil || event.DerpLatency != nil || event.P2PLatency != nil

	if hasNetworkInfo {
		// Apply explicit values from the event.
		if event.P2PEndpoint != nil {
			p2p := true
			entry.P2P = &p2p
		}
		if event.DerpLatency != nil {
			d := event.DerpLatency.AsDuration()
			entry.DERPLatency = &d
			p2p := false
			entry.P2P = &p2p
		}
		if event.P2PLatency != nil {
			d := event.P2PLatency.AsDuration()
			entry.P2PLatency = &d
			p2p := true
			entry.P2P = &p2p
		}
	} else if existing != nil {
		// Event has no mode/latency info — preserve prior values
		// so a bare CONNECTED event doesn't wipe known state.
		entry.P2P = existing.P2P
		entry.DERPLatency = existing.DERPLatency
		entry.P2PLatency = existing.P2PLatency
	}

	byPeer[peerID] = entry
}

func (s *PeerNetworkTelemetryStore) Get(agentID uuid.UUID, peerID ...uuid.UUID) *PeerNetworkTelemetry {
	if len(peerID) > 0 {
		return s.getByPeer(agentID, peerID[0])
	}

	// Legacy callers only provide agentID. Return the freshest peer entry.
	entries := s.GetAll(agentID)
	var latest *PeerNetworkTelemetry
	for _, entry := range entries {
		if latest == nil || entry.LastUpdatedAt.After(latest.LastUpdatedAt) {
			latest = entry
		}
	}
	return latest
}

func (s *PeerNetworkTelemetryStore) getByPeer(agentID, peerID uuid.UUID) *PeerNetworkTelemetry {
	s.mu.Lock()
	defer s.mu.Unlock()

	byPeer := s.byAgent[agentID]
	if byPeer == nil {
		return nil
	}

	entry := byPeer[peerID]
	if entry == nil {
		return nil
	}
	if time.Since(entry.LastUpdatedAt) > peerNetworkTelemetryMaxAge {
		delete(byPeer, peerID)
		if len(byPeer) == 0 {
			delete(s.byAgent, agentID)
		}
		return nil
	}
	return entry
}

func (s *PeerNetworkTelemetryStore) GetAll(agentID uuid.UUID) map[uuid.UUID]*PeerNetworkTelemetry {
	s.mu.Lock()
	defer s.mu.Unlock()

	byPeer := s.byAgent[agentID]
	if len(byPeer) == 0 {
		return nil
	}

	entries := make(map[uuid.UUID]*PeerNetworkTelemetry, len(byPeer))
	now := time.Now()
	for peerID, entry := range byPeer {
		if now.Sub(entry.LastUpdatedAt) > peerNetworkTelemetryMaxAge {
			delete(byPeer, peerID)
			continue
		}
		entries[peerID] = entry
	}

	if len(byPeer) == 0 {
		delete(s.byAgent, agentID)
	}
	if len(entries) == 0 {
		return nil
	}
	return entries
}

func (s *PeerNetworkTelemetryStore) Delete(agentID, peerID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byPeer := s.byAgent[agentID]
	if byPeer == nil {
		return
	}
	delete(byPeer, peerID)
	if len(byPeer) == 0 {
		delete(s.byAgent, agentID)
	}
}
