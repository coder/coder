package agentapi

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/proto"
)

type ResourcesMonitoringAPI struct {
}

func (a *ResourcesMonitoringAPI) GetResourcesMonitoringConfiguration(ctx context.Context, _ *proto.GetResourcesMonitoringConfigurationRequest) (*proto.GetResourcesMonitoringConfigurationResponse, error) {
	return nil, xerrors.Errorf("GetResourcesMonitoringConfiguration is not implemented")
}

func (a *ResourcesMonitoringAPI) PushResourcesMonitoringUsage(ctx context.Context, _ *proto.PushResourcesMonitoringUsageRequest) (*proto.PushResourcesMonitoringUsageResponse, error) {
	return nil, xerrors.Errorf("PushResourcesMonitoringUsage is not implemented")
}
