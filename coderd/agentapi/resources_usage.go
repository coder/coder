package agentapi

import (
	"context"
	"fmt"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/notifications"
)

type ResourcesUsageAPI struct {
	WorkspaceFn           func(context.Context) (database.Workspace, error)
	Database              database.Store
	NotificationsEnqueuer notifications.Enqueuer
	Log                   slog.Logger
}

var (
	// hardcoded test percentages - testing purpose
	memoryThreshold int32 = 90
	diskThreshold   int32 = 20
)

func (a *ResourcesUsageAPI) PushResourcesUsage(ctx context.Context, req *proto.PushResourcesUsageRequest) (*proto.PushResourcesUsageResponse, error) {
	a.Log.Info(ctx, "push resources usage", slog.F("request", req))

	workspace, err := a.WorkspaceFn(ctx)
	if err != nil {
		a.Log.Info(ctx, "push resources failed - WorkspaceFn", slog.Error(err))
		return &proto.PushResourcesUsageResponse{}, err
	}

	// TODO : Implement this logic to fetch thresholds from build parameters.

	// build, err := a.Database.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	// if err != nil {
	// 	a.Log.Info(ctx, "push resources failed - GetBuild", slog.Error(err))
	// 	return &proto.PushResourcesUsageResponse{}, err
	// }

	// params, err := a.Database.GetWorkspaceBuildParameters(ctx, build.ID)
	// if err != nil {
	// 	a.Log.Info(ctx, "push resources failed - GetParameters", slog.Error(err))
	// 	return &proto.PushResourcesUsageResponse{}, err
	// }

	// Implement logic required for debouncing
	if req.PercentDisk >= diskThreshold {
		_, err = a.NotificationsEnqueuer.Enqueue(dbauthz.AsNotifier(ctx), workspace.OwnerID, notifications.TemplateResourceThresholdReached, map[string]string{
			"resource":           "disk",
			"resource_threshold": fmt.Sprintf("%v", diskThreshold),
			"workspace_name":     workspace.Name,
		}, "agent", workspace.OwnerID)
		a.Log.Info(ctx, "push resources final", slog.Error(err))
	}

	if req.PercentMemory >= memoryThreshold {
		_, err = a.NotificationsEnqueuer.Enqueue(dbauthz.AsNotifier(ctx), workspace.OwnerID, notifications.TemplateResourceThresholdReached, map[string]string{
			"resource":           "memory",
			"resource_threshold": fmt.Sprintf("%v", memoryThreshold),
			"workspace_name":     workspace.Name,
		}, "agent", workspace.OwnerID)
		a.Log.Info(ctx, "push resources final", slog.Error(err))
	}

	return &proto.PushResourcesUsageResponse{}, nil
}
