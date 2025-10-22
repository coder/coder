package tailnet

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/tailnet/proto"
)

var legacyWorkspaceAgentIP = netip.MustParseAddr("fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4")

type InvalidAddressBitsError struct {
	Bits int
}

func (e InvalidAddressBitsError) Error() string {
	return fmt.Sprintf("invalid address bits, expected 128, got %d", e.Bits)
}

type InvalidNodeAddressError struct {
	Addr string
}

func (e InvalidNodeAddressError) Error() string {
	return fmt.Sprintf("invalid node address, got %s", e.Addr)
}

type CoordinateeAuth interface {
	Authorize(ctx context.Context, req *proto.CoordinateRequest) error
}

// SingleTailnetCoordinateeAuth allows all tunnels, since Coderd and wsproxy are allowed to initiate a tunnel to any agent
type SingleTailnetCoordinateeAuth struct{}

func (SingleTailnetCoordinateeAuth) Authorize(context.Context, *proto.CoordinateRequest) error {
	return nil
}

// ClientCoordinateeAuth allows connecting to a single, given agent
type ClientCoordinateeAuth struct {
	AgentID uuid.UUID
}

func (c ClientCoordinateeAuth) Authorize(_ context.Context, req *proto.CoordinateRequest) error {
	if tun := req.GetAddTunnel(); tun != nil {
		uid, err := uuid.FromBytes(tun.Id)
		if err != nil {
			return xerrors.Errorf("parse add tunnel id: %w", err)
		}

		if c.AgentID != uid {
			return xerrors.Errorf("invalid agent id, expected %s, got %s", c.AgentID.String(), uid.String())
		}
	}

	return handleClientNodeRequests(req)
}

// AgentCoordinateeAuth disallows all tunnels, since agents are not allowed to initiate their own tunnels
type AgentCoordinateeAuth struct {
	ID uuid.UUID
}

func (a AgentCoordinateeAuth) Authorize(_ context.Context, req *proto.CoordinateRequest) error {
	if tun := req.GetAddTunnel(); tun != nil {
		return xerrors.New("agents cannot open tunnels")
	}

	if upd := req.GetUpdateSelf(); upd != nil {
		for _, addrStr := range upd.Node.Addresses {
			pre, err := netip.ParsePrefix(addrStr)
			if err != nil {
				return xerrors.Errorf("parse node address: %w", err)
			}

			if pre.Bits() != 128 {
				return InvalidAddressBitsError{pre.Bits()}
			}

			if TailscaleServicePrefix.AddrFromUUID(a.ID).Compare(pre.Addr()) != 0 &&
				CoderServicePrefix.AddrFromUUID(a.ID).Compare(pre.Addr()) != 0 &&
				legacyWorkspaceAgentIP.Compare(pre.Addr()) != 0 {
				return InvalidNodeAddressError{pre.Addr().String()}
			}
		}
	}

	return nil
}

type ClientUserCoordinateeAuth struct {
	Auth TunnelAuthorizer
}

func (a ClientUserCoordinateeAuth) Authorize(ctx context.Context, req *proto.CoordinateRequest) error {
	if tun := req.GetAddTunnel(); tun != nil {
		uid, err := uuid.FromBytes(tun.Id)
		if err != nil {
			return xerrors.Errorf("parse add tunnel id: %w", err)
		}
		err = a.Auth.AuthorizeTunnel(ctx, uid)
		if err != nil {
			return xerrors.Errorf("workspace agent not found or you do not have permission")
		}
	}

	return handleClientNodeRequests(req)
}

// handleClientNodeRequests validates GetUpdateSelf requests and declines ReadyForHandshake requests
func handleClientNodeRequests(req *proto.CoordinateRequest) error {
	if upd := req.GetUpdateSelf(); upd != nil {
		for _, addrStr := range upd.Node.Addresses {
			pre, err := netip.ParsePrefix(addrStr)
			if err != nil {
				return xerrors.Errorf("parse node address: %w", err)
			}

			if pre.Bits() != 128 {
				return InvalidAddressBitsError{pre.Bits()}
			}
		}
	}

	if rfh := req.GetReadyForHandshake(); rfh != nil {
		return xerrors.Errorf("clients may not send ready_for_handshake")
	}
	return nil
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

func (s *tunnelStore) tunnelExists(src, dst uuid.UUID) bool {
	_, srcOK := s.bySrc[src][dst]
	_, dstOK := s.byDst[src][dst]
	return srcOK || dstOK
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
