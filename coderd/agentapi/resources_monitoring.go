package agentapi

import (
	"context"
	"database/sql"
	"errors"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type ResourcesMonitoringAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store
	Log      slog.Logger
}

func (a *ResourcesMonitoringAPI) GetResourcesMonitoringConfiguration(ctx context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
	agent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch agent: %w", err)
	}

	memoryMonitor, memoryErr := a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, agent.ID)
	if memoryErr != nil && !errors.Is(memoryErr, sql.ErrNoRows) {
		return nil, xerrors.Errorf("failed to fetch memory resource monitor: %w", memoryErr)
	}

	volumeMonitors, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, agent.ID)
	if err != nil {
		return nil, xerrors.Errorf("failed to fetch volume resource monitors: %w", err)
	}

	return &proto.GetResourcesMonitoringConfigurationResponse{
		Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
			CollectionIntervalSeconds: 10,
			NumDatapoints:             20,
		},
		Memory: func() *proto.GetResourcesMonitoringConfigurationResponse_Memory {
			if memoryErr != nil {
				return nil
			}

			return &proto.GetResourcesMonitoringConfigurationResponse_Memory{
				Enabled: memoryMonitor.Enabled,
			}
		}(),
		Volumes: func() []*proto.GetResourcesMonitoringConfigurationResponse_Volume {
			volumes := make([]*proto.GetResourcesMonitoringConfigurationResponse_Volume, 0, len(volumeMonitors))
			for _, monitor := range volumeMonitors {
				volumes = append(volumes, &proto.GetResourcesMonitoringConfigurationResponse_Volume{
					Enabled: monitor.Enabled,
					Path:    monitor.Path,
				})
			}

			return volumes
		}(),
	}, nil
}

func (a *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
	a.Log.Info(ctx, "resources monitoring usage received",
		slog.F("request", req))

	return &proto.PushResourcesMonitoringUsageResponse{}, nil
}
