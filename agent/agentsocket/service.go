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
	ErrUnitNameRequired        = xerrors.New("unit name is required")
)

type DRPCAgentSocketService struct {
	unitManager *unit.Manager[string]
	logger      slog.Logger
}

func (*DRPCAgentSocketService) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncStart(_ context.Context, req *proto.SyncStartRequest) (*proto.SyncStartResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStartResponse{}, nil
	}

	if req.Unit == "" {
		return &proto.SyncStartResponse{}, nil
	}

	if err := s.unitManager.Register(req.Unit); err != nil {
		// If already registered, that's okay - we can still update status
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return &proto.SyncStartResponse{}, nil
		}
	}

	isReady, err := s.unitManager.IsReady(req.Unit)
	switch {
	case err != nil:
		return &proto.SyncStartResponse{}, xerrors.Errorf("failed to check readiness: %w", err)
	case !isReady:
		return &proto.SyncStartResponse{}, xerrors.Errorf("unit not ready: %q", req.Unit)
	}

	err = s.unitManager.UpdateStatus(req.Unit, unit.StatusStarted)
	switch {
	case errors.Is(err, unit.ErrSameStatusAlreadySet):
		return &proto.SyncStartResponse{}, xerrors.Errorf("unit already started: %q", req.Unit)
	case err != nil:
		return &proto.SyncStartResponse{}, xerrors.Errorf("failed to update status: %w", err)
	}

	return &proto.SyncStartResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncWant(_ context.Context, req *proto.SyncWantRequest) (*proto.SyncWantResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncWantResponse{}, nil
	}

	if req.Unit == "" || req.DependsOn == "" {
		return &proto.SyncWantResponse{}, nil
	}

	if err := s.unitManager.Register(req.Unit); err != nil {
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return &proto.SyncWantResponse{}, nil
		}
	}

	if err := s.unitManager.Register(req.DependsOn); err != nil {
		if !errors.Is(err, unit.ErrUnitAlreadyRegistered) {
			return &proto.SyncWantResponse{}, nil
		}
	}

	if err := s.unitManager.AddDependency(req.Unit, req.DependsOn, unit.StatusComplete); err != nil {
		return &proto.SyncWantResponse{}, nil
	}

	return &proto.SyncWantResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncComplete(_ context.Context, req *proto.SyncCompleteRequest) (*proto.SyncCompleteResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncCompleteResponse{}, nil
	}

	if req.Unit == "" {
		return &proto.SyncCompleteResponse{}, nil
	}

	if err := s.unitManager.UpdateStatus(req.Unit, unit.StatusComplete); err != nil {
		return &proto.SyncCompleteResponse{}, nil
	}

	return &proto.SyncCompleteResponse{}, nil
}

func (s *DRPCAgentSocketService) SyncReady(_ context.Context, req *proto.SyncReadyRequest) (*proto.SyncReadyResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncReadyResponse{}, nil
	}

	if req.Unit == "" {
		return &proto.SyncReadyResponse{}, nil
	}

	isReady, err := s.unitManager.IsReady(req.Unit)
	switch {
	case !isReady || errors.Is(err, unit.ErrUnitNotFound):
		return &proto.SyncReadyResponse{}, xerrors.Errorf("unit not ready: %q", req.Unit)
	case err != nil:
		return &proto.SyncReadyResponse{}, xerrors.Errorf("failed to check readiness: %w", err)
	default:
		return &proto.SyncReadyResponse{}, nil
	}
}

func (s *DRPCAgentSocketService) SyncStatus(_ context.Context, req *proto.SyncStatusRequest) (*proto.SyncStatusResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: ErrUnitManagerNotAvailable.Error(),
		}, nil
	}

	if req.Unit == "" {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: ErrUnitNameRequired.Error(),
		}, nil
	}

	status, err := s.unitManager.GetStatus(req.Unit)
	if err != nil {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: "failed to get unit status: " + err.Error(),
		}, nil
	}

	isReady, err := s.unitManager.IsReady(req.Unit)
	if err != nil {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: "failed to check readiness: " + err.Error(),
		}, nil
	}

	dependencies, err := s.unitManager.GetAllDependencies(req.Unit)
	if err != nil {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: "failed to get dependencies: " + err.Error(),
		}, nil
	}

	var depInfos []*proto.DependencyInfo
	for _, dep := range dependencies {
		depInfos = append(depInfos, &proto.DependencyInfo{
			DependsOn:      dep.DependsOn,
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

	return &proto.SyncStatusResponse{
		Success:      true,
		Message:      "unit status retrieved successfully",
		Unit:         req.Unit,
		Status:       string(status),
		IsReady:      isReady,
		Dependencies: depInfos,
		Dot:          dotStr,
	}, nil
}
