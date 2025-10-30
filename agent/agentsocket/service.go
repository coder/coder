package agentsocket

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
)

// DRPCAgentSocketService implements the proto.DRPCAgentSocketServer interface.
type DRPCAgentSocketService struct {
	mu          sync.RWMutex
	unitManager *unit.Manager[string, string]
	logger      slog.Logger
}

func (*DRPCAgentSocketService) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{
		Message:   "pong",
		Timestamp: timestamppb.New(time.Now()),
	}, nil
}

func (s *DRPCAgentSocketService) SyncStart(_ context.Context, req *proto.SyncStartRequest) (*proto.SyncStartResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStartResponse{
			Success: false,
			Message: "dependency tracker not available",
		}, nil
	}

	if req.Unit == "" {
		return &proto.SyncStartResponse{
			Success: false,
			Message: "Unit name is required",
		}, nil
	}

	if err := s.unitManager.Register(req.Unit); err != nil {
		// If already registered, that's okay - we can still update status
		if !errors.Is(err, unit.ErrConsumerAlreadyRegistered) {
			return &proto.SyncStartResponse{
				Success: false,
				Message: "Failed to register unit: " + err.Error(),
			}, nil
		}
	}

	isReady, err := s.unitManager.IsReady(req.Unit)
	if err != nil {
		return &proto.SyncStartResponse{
			Success: false,
			Message: "Failed to check readiness: " + err.Error(),
		}, nil
	}
	if !isReady {
		return &proto.SyncStartResponse{
			Success: false,
			Message: "Unit is not ready",
		}, nil
	}

	if err := s.unitManager.UpdateStatus(req.Unit, StatusStarted); err != nil {
		return &proto.SyncStartResponse{
			Success: false,
			Message: "Failed to update status: " + err.Error(),
		}, nil
	}

	return &proto.SyncStartResponse{
		Success: true,
		Message: "Unit " + req.Unit + " started successfully",
	}, nil
}

func (s *DRPCAgentSocketService) SyncWant(_ context.Context, req *proto.SyncWantRequest) (*proto.SyncWantResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncWantResponse{
			Success: false,
			Message: "unit manager not available",
		}, nil
	}

	if req.Unit == "" || req.DependsOn == "" {
		return &proto.SyncWantResponse{
			Success: false,
			Message: "unit and depends_on are required",
		}, nil
	}

	if err := s.unitManager.Register(req.Unit); err != nil {
		if !errors.Is(err, unit.ErrConsumerAlreadyRegistered) {
			return &proto.SyncWantResponse{
				Success: false,
				Message: "failed to register unit: " + err.Error(),
			}, nil
		}
	}

	if err := s.unitManager.Register(req.DependsOn); err != nil {
		if !errors.Is(err, unit.ErrConsumerAlreadyRegistered) {
			return &proto.SyncWantResponse{
				Success: false,
				Message: "failed to register dependency unit: " + err.Error(),
			}, nil
		}
	}

	if err := s.unitManager.AddDependency(req.Unit, req.DependsOn, StatusComplete); err != nil {
		return &proto.SyncWantResponse{
			Success: false,
			Message: "failed to add dependency: " + err.Error(),
		}, nil
	}

	return &proto.SyncWantResponse{
		Success: true,
		Message: "Unit " + req.Unit + " now depends on " + req.DependsOn,
	}, nil
}

func (s *DRPCAgentSocketService) SyncComplete(_ context.Context, req *proto.SyncCompleteRequest) (*proto.SyncCompleteResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncCompleteResponse{
			Success: false,
			Message: "unit manager not available",
		}, nil
	}

	if req.Unit == "" {
		return &proto.SyncCompleteResponse{
			Success: false,
			Message: "unit name is required",
		}, nil
	}

	if err := s.unitManager.UpdateStatus(req.Unit, StatusComplete); err != nil {
		return &proto.SyncCompleteResponse{
			Success: false,
			Message: "failed to update status: " + err.Error(),
		}, nil
	}

	return &proto.SyncCompleteResponse{
		Success: true,
		Message: "unit " + req.Unit + " completed successfully",
	}, nil
}

func (s *DRPCAgentSocketService) SyncWait(_ context.Context, req *proto.SyncWaitRequest) (*proto.SyncWaitResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncWaitResponse{
			Success: false,
			Message: "unit manager not available",
		}, nil
	}

	if req.Unit == "" {
		return &proto.SyncWaitResponse{
			Success: false,
			Message: "unit name is required",
		}, nil
	}

	isReady, err := s.unitManager.IsReady(req.Unit)
	if err != nil {
		return &proto.SyncWaitResponse{
			Success: false,
			Message: "failed to check readiness: " + err.Error(),
		}, nil
	}

	if !isReady {
		return &proto.SyncWaitResponse{
			Success: false,
			Message: unit.ErrDependenciesNotSatisfied.Error(),
		}, nil
	}

	return &proto.SyncWaitResponse{
		Success: true,
		Message: "unit " + req.Unit + " dependencies are satisfied",
	}, nil
}

func (s *DRPCAgentSocketService) SyncStatus(_ context.Context, req *proto.SyncStatusRequest) (*proto.SyncStatusResponse, error) {
	if s.unitManager == nil {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: "unit manager not available",
		}, nil
	}

	if req.Unit == "" {
		return &proto.SyncStatusResponse{
			Success: false,
			Message: "unit name is required",
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
			RequiredStatus: dep.RequiredStatus,
			CurrentStatus:  dep.CurrentStatus,
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
		Status:       status,
		IsReady:      isReady,
		Dependencies: depInfos,
		Dot:          dotStr,
	}, nil
}
