package tailnet

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	agpl "github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

const exitNodeAgentCacheDuration = 5 * time.Second

// ExitNodeCoordinateeAuth restricts a replica to its identity and bound agents.
type ExitNodeCoordinateeAuth struct {
	Database   database.Store
	Clock      quartz.Clock
	ExitNodeID uuid.UUID
	PeerID     uuid.UUID

	mu        sync.Mutex
	agentIDs  map[uuid.UUID]struct{}
	refreshed time.Time
}

// Authorize validates tunnel and self-update requests.
func (a *ExitNodeCoordinateeAuth) Authorize(ctx context.Context, req *proto.CoordinateRequest) error {
	if req.GetReadyForHandshake() != nil {
		return xerrors.New("exit nodes may not send ready_for_handshake")
	}
	if update := req.GetUpdateSelf(); update != nil {
		if err := a.authorizeNodePrefixes(update.Node.GetAddresses()); err != nil {
			return xerrors.Errorf("addresses: %w", err)
		}
		if err := a.authorizeNodePrefixes(update.Node.GetAllowedIps()); err != nil {
			return xerrors.Errorf("allowed IPs: %w", err)
		}
	}
	if tun := req.GetAddTunnel(); tun != nil {
		if err := a.authorizeTunnel(ctx, tun.Id); err != nil {
			return err
		}
	}
	if tun := req.GetRemoveTunnel(); tun != nil {
		if _, err := uuid.FromBytes(tun.Id); err != nil {
			return xerrors.Errorf("parse tunnel agent id: %w", err)
		}
	}
	return nil
}

func (a *ExitNodeCoordinateeAuth) authorizeNodePrefixes(prefixes []string) error {
	expected := agpl.TailscaleServicePrefix.AddrFromUUID(a.PeerID)
	for _, prefixString := range prefixes {
		prefix, err := netip.ParsePrefix(prefixString)
		if err != nil {
			return xerrors.Errorf("parse node address: %w", err)
		}
		if prefix.Bits() != 128 || prefix.Addr() != expected {
			return xerrors.Errorf("invalid exit node address %s", prefix)
		}
	}
	return nil
}

func (a *ExitNodeCoordinateeAuth) authorizeTunnel(ctx context.Context, rawID []byte) error {
	agentID, err := uuid.FromBytes(rawID)
	if err != nil {
		return xerrors.Errorf("parse tunnel agent id: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.Clock.Now("exit_node_coordinate_auth")
	_, found := a.agentIDs[agentID]
	if a.agentIDs == nil || !found || now.Sub(a.refreshed) > exitNodeAgentCacheDuration {
		//nolint:gocritic // Binding lookup spans workspaces.
		agentIDs, err := a.Database.GetWorkspaceAgentIDsByExitNode(dbauthz.AsSystemRestricted(ctx), a.ExitNodeID)
		if err != nil {
			return xerrors.Errorf("get exit node agents: %w", err)
		}
		a.agentIDs = make(map[uuid.UUID]struct{}, len(agentIDs))
		for _, id := range agentIDs {
			a.agentIDs[id] = struct{}{}
		}
		a.refreshed = now
		_, found = a.agentIDs[agentID]
	}
	if !found {
		return xerrors.Errorf("agent %s is not bound to exit node %s", agentID, a.ExitNodeID)
	}
	return nil
}
