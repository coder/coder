package agentapi

import (
	"context"
	"database/sql"
	"errors"

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
		return nil, err
	}

	_, err = a.Database.FetchMemoryResourceMonitorsByAgentID(ctx, agent.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	volumeMonitors, err := a.Database.FetchVolumesResourceMonitorsByAgentID(ctx, agent.ID)
	if err != nil {
		return nil, err
	}

	volumes := make([]string, 0, len(volumeMonitors))
	for _, monitor := range volumeMonitors {
		volumes = append(volumes, monitor.Path)
	}

	return &proto.GetResourcesMonitoringConfigurationResponse{
		Enabled: false,
		Config: &proto.GetResourcesMonitoringConfigurationResponse_Config{
			CollectionIntervalSeconds: 10,
			NumDatapoints:             20,
		},
		MonitoredVolumes: volumes,
	}, nil
}

func (a *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, req *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
	a.Log.Info(ctx, "resources monitoring usage received",
		slog.F("request", req))

	return &proto.PushResourcesMonitoringUsageResponse{}, nil
}
