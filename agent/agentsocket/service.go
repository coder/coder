package agentsocket

import (
	"context"
	"errors"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
)

var _ proto.DRPCAgentSocketServer = (*DRPCAgentSocketService)(nil)

var (
	ErrUnitManagerNotAvailable = xerrors.New("unit manager not available")
)

type DRPCAgentSocketService struct {
	unitManager *unit.Manager
	logger      slog.Logger
}

func (*DRPCAgentSocketService) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncStart(_ context.Context, req *proto.SyncStartRequest) (*proto.SyncStartResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStartResponse{}, xerrors.Errorf("SyncStart: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.Register(unitID); err != nil {
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return &proto.SyncStartResponse{}, xerrors.Errorf("SyncStart: %w", err)
		}
	}

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return &proto.SyncStartResponse{}, xerrors.Errorf("cannot check readiness: %w", err)
	}
	if !isReady {
		return &proto.SyncStartResponse{}, xerrors.Errorf("cannot start unit %q: unit not ready", req.Unit)
	}

	err = s.unitManager.UpdateStatus(unitID, unit.StatusStarted)
	if err != nil {
		return &proto.SyncStartResponse{}, xerrors.Errorf("cannot start unit %q: %w", req.Unit, err)
	}

	return &proto.SyncStartResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncWant(_ context.Context, req *proto.SyncWantRequest) (*proto.SyncWantResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncWantResponse{}, xerrors.Errorf("cannot add dependency: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	dependsOnID := unit.ID(req.DependsOn)

	if err := s.unitManager.Register(unitID); err != nil && !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
		return &proto.SyncWantResponse{}, xerrors.Errorf("cannot add dependency: %w", err)
	}

	if err := s.unitManager.AddDependency(unitID, dependsOnID, unit.StatusComplete); err != nil {
		return &proto.SyncWantResponse{}, xerrors.Errorf("cannot add dependency: %w", err)
	}

	return &proto.SyncWantResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncComplete(_ context.Context, req *proto.SyncCompleteRequest) (*proto.SyncCompleteResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncCompleteResponse{}, xerrors.Errorf("cannot complete unit: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	if err := s.unitManager.UpdateStatus(unitID, unit.StatusComplete); err != nil {
		return &proto.SyncCompleteResponse{}, xerrors.Errorf("cannot complete unit %q: %w", req.Unit, err)
	}

	return &proto.SyncCompleteResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncReady(_ context.Context, req *proto.SyncReadyRequest) (*proto.SyncReadyResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncReadyResponse{}, xerrors.Errorf("cannot check readiness: %w", ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)
	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return &proto.SyncReadyResponse{}, xerrors.Errorf("cannot check readiness: %w", err)
	}
	if !isReady {
		return &proto.SyncReadyResponse{}, xerrors.Errorf("unit not ready: %q", req.Unit)
	}

	return &proto.SyncReadyResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncStatus(_ context.Context, req *proto.SyncStatusRequest) (*proto.SyncStatusResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStatusResponse{}, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, ErrUnitManagerNotAvailable)
	}

	unitID := unit.ID(req.Unit)

	isReady, err := s.unitManager.IsReady(unitID)
	if err != nil {
		return &proto.SyncStatusResponse{}, xerrors.Errorf("cannot check readiness: %w", err)
	}

	dependencies, err := s.unitManager.GetAllDependencies(unitID)
	if err != nil {
		return &proto.SyncStatusResponse{}, xerrors.Errorf("failed to get dependencies: %w", err)
	}

	var depInfos []*proto.DependencyInfo
	for _, dep := range dependencies {
		depInfos = append(depInfos, &proto.DependencyInfo{
			DependsOn:      string(dep.DependsOn),
			RequiredStatus: string(dep.RequiredStatus),
			CurrentStatus:  string(dep.CurrentStatus),
			IsSatisfied:    dep.IsSatisfied,
		})
	}

	var dotStr string
	if req.Recursive {
		dotStr, err = s.unitManager.ExportDOT("dependency_graph")
		if err != nil {
			return &proto.SyncStatusResponse{
				Success: false,
				Message: "failed to export DOT: " + err.Error(),
			}, nil
		}
	}

	u, err := s.unitManager.Unit(unitID)
	if err != nil {
		return &proto.SyncStatusResponse{}, xerrors.Errorf("cannot get status for unit %q: %w", req.Unit, err)
	}
	return &proto.SyncStatusResponse{
		Success:      true,
		Message:      "unit status retrieved successfully",
		Unit:         req.Unit,
		Status:       string(u.Status()),
		IsReady:      isReady,
		Dependencies: depInfos,
		Dot:          dotStr,
	}, nil
}
