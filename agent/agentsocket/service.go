package agentsocket

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/agent/unit"
)

var _ proto.DRPCAgentSocketServer = (*DRPCAgentSocketService)(nil)

var (
	ErrUnitManagerNotAvailable = xerrors.New("unit manager not available")
	ErrAgentAPINotConnected    = xerrors.New("agent not connected to coderd")
)

// DRPCAgentSocketService implements the DRPC agent socket service.
type DRPCAgentSocketService struct {
	unitManager *unit.Manager
	logger      slog.Logger

	mu       sync.Mutex
	agentAPI agentproto.DRPCAgentClient28
}

// SetAgentAPI sets the agent API client used to forward requests
// to coderd. This is called when the agent connects to coderd.
func (s *DRPCAgentSocketService) SetAgentAPI(api agentproto.DRPCAgentClient28) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAPI = api
}

// ClearAgentAPI clears the agent API client. This is called when
// the agent disconnects from coderd.
func (s *DRPCAgentSocketService) ClearAgentAPI() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAPI = nil
}

// Ping responds to a ping request to check if the service is alive.
func (*DRPCAgentSocketService) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

// SyncStart starts a unit in the dependency graph.
func (s *DRPCAgentSocketService) SyncStart(_ context.Context, req *proto.SyncStartRequest) (*proto.SyncStartResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("SyncStart: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.Register(unitID); err != nil {
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return nil, xerrors.Errorf("SyncStart: %w", err)
		}
	}

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}
	if !isReady {
		return nil, xerrors.Errorf("cannot start unit %q: unit not ready", req.Unit)
	}

	err = s.unitManager.UpdateStatus(unitID, unit.StatusStarted)
	if err != nil {
		return nil, xerrors.Errorf("cannot start unit %q: %w", req.Unit, err)
	}

	return &proto.SyncStartResponse{}, nil
}

// SyncWant declares a dependency between units.
func (s *DRPCAgentSocketService) SyncWant(_ context.Context, req *proto.SyncWantRequest) (*proto.SyncWantResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot add dependency: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	dependsOnID := unit.ID(req.DependsOn)

	if err := s.unitManager.Register(unitID); err != nil && !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
		return nil, xerrors.Errorf("cannot add dependency: %w", err)
	}

	if err := s.unitManager.AddDependency(unitID, dependsOnID, unit.StatusComplete); err != nil {
		return nil, xerrors.Errorf("cannot add dependency: %w", err)
	}

	return &proto.SyncWantResponse{}, nil
}

// SyncComplete marks a unit as complete in the dependency graph.
func (s *DRPCAgentSocketService) SyncComplete(_ context.Context, req *proto.SyncCompleteRequest) (*proto.SyncCompleteResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot complete unit: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.UpdateStatus(unitID, unit.StatusComplete); err != nil {
		return nil, xerrors.Errorf("cannot complete unit %q: %w", req.Unit, err)
	}

	return &proto.SyncCompleteResponse{}, nil
}

// SyncReady checks whether a unit is ready to be started. That is, all dependencies are satisfied.
func (s *DRPCAgentSocketService) SyncReady(_ context.Context, req *proto.SyncReadyRequest) (*proto.SyncReadyResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}

	return &proto.SyncReadyResponse{
		Ready: isReady,
	}, nil
}

// SyncStatus gets the status of a unit and lists its dependencies.
func (s *DRPCAgentSocketService) SyncStatus(_ context.Context, req *proto.SyncStatusRequest) (*proto.SyncStatusResponse, error) {
	if s.unitManager == nil {
		return nil, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot check readiness: %w", err)
	}

	dependencies, err := s.unitManager.GetAllDependencies(unitID)
	switch {
	case errors.Is(err, unit.ErrUnitNotFound):
		dependencies = []unit.Dependency{}
	case err != nil:
		return nil, xerrors.Errorf("cannot get dependencies: %w", err)
	}

	var depInfos []*proto.DependencyInfo
	for _, dep := range dependencies {
		depInfos = append(depInfos, &proto.DependencyInfo{
			Unit:           string(dep.Unit),
			DependsOn:      string(dep.DependsOn),
			RequiredStatus: string(dep.RequiredStatus),
			CurrentStatus:  string(dep.CurrentStatus),
			IsSatisfied:    dep.IsSatisfied,
		})
	}

	u, err := s.unitManager.Unit(unitID)
	if err != nil {
		return nil, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, err)
	}
	return &proto.SyncStatusResponse{
		Status:       string(u.Status()),
		IsReady:      isReady,
		Dependencies: depInfos,
	}, nil
}

// UpdateAppStatus forwards an app status update to coderd via the
// agent API. Returns an error if the agent is not connected.
func (s *DRPCAgentSocketService) UpdateAppStatus(ctx context.Context, req *agentproto.UpdateAppStatusRequest) (*agentproto.UpdateAppStatusResponse, error) {
	s.mu.Lock()
	api := s.agentAPI
	s.mu.Unlock()

	if api == nil {
		return nil, ErrAgentAPINotConnected
	}
	return api.UpdateAppStatus(ctx, req)
}
