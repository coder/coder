package tailnet

import "github.com/google/uuid"

type TunnelAuth interface {
	Authorize(dst uuid.UUID) bool
}

// SingleTailnetTunnelAuth allows all tunnels, since Coderd and wsproxy are allowed to initiate a tunnel to any agent
type SingleTailnetTunnelAuth struct{}

func (SingleTailnetTunnelAuth) Authorize(uuid.UUID) bool {
	return true
}

// ClientTunnelAuth allows connecting to a single, given agent
type ClientTunnelAuth struct {
	AgentID uuid.UUID
}

func (c ClientTunnelAuth) Authorize(dst uuid.UUID) bool {
	return c.AgentID == dst
}

// AgentTunnelAuth disallows all tunnels, since agents are not allowed to initiate their own tunnels
type AgentTunnelAuth struct{}

func (AgentTunnelAuth) Authorize(uuid.UUID) bool {
	return false
}

// tunnelStore contains tunnel information and allows querying it.  It is not threadsafe and all
// methods must be serialized by holding, e.g. the core mutex.
type tunnelStore struct {
	bySrc map[uuid.UUID]map[uuid.UUID]struct{}
	byDst map[uuid.UUID]map[uuid.UUID]struct{}
}

func newTunnelStore() *tunnelStore {
	return &tunnelStore{
		bySrc: make(map[uuid.UUID]map[uuid.UUID]struct{}),
		byDst: make(map[uuid.UUID]map[uuid.UUID]struct{}),
	}
}

func (s *tunnelStore) add(src, dst uuid.UUID) {
	srcM, ok := s.bySrc[src]
	if !ok {
		srcM = make(map[uuid.UUID]struct{})
		s.bySrc[src] = srcM
	}
	srcM[dst] = struct{}{}
	dstM, ok := s.byDst[dst]
	if !ok {
		dstM = make(map[uuid.UUID]struct{})
		s.byDst[dst] = dstM
	}
	dstM[src] = struct{}{}
}

func (s *tunnelStore) remove(src, dst uuid.UUID) {
	delete(s.bySrc[src], dst)
	if len(s.bySrc[src]) == 0 {
		delete(s.bySrc, src)
	}
	delete(s.byDst[dst], src)
	if len(s.byDst[dst]) == 0 {
		delete(s.byDst, dst)
	}
}

func (s *tunnelStore) removeAll(src uuid.UUID) {
	for dst := range s.bySrc[src] {
		s.remove(src, dst)
	}
}

func (s *tunnelStore) findTunnelPeers(id uuid.UUID) []uuid.UUID {
	set := make(map[uuid.UUID]struct{})
	for dst := range s.bySrc[id] {
		set[dst] = struct{}{}
	}
	for src := range s.byDst[id] {
		set[src] = struct{}{}
	}
	out := make([]uuid.UUID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (s *tunnelStore) htmlDebug() []HTMLTunnel {
	out := make([]HTMLTunnel, 0)
	for src, dsts := range s.bySrc {
		for dst := range dsts {
			out = append(out, HTMLTunnel{Src: src, Dst: dst})
		}
	}
	return out
}
