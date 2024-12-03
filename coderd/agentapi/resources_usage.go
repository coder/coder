package agentapi

import (
	"context"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

type ResourcesUsageAPI struct {
	AgentFn  func(context.Context) (database.WorkspaceAgent, error)
	Database database.Store
	Log      slog.Logger
}

var (
	// hardcoded test percentages - testing purpose
	memoryThreshold = 90
	diskThreshold   = 20
)

func (a *ResourcesUsageAPI) PushResourcesUsage(ctx context.Context, req *proto.PushResourcesUsageRequest) (*proto.PushResourcesUsageResponse, error) {
	a.Log.Info(ctx, "push resources usage", slog.F("request", req))

	// TODO : Implement this logic to fetch thresholds from build parameters.
	// workspaceAgent, err := a.AgentFn(ctx)
	// if err != nil {
	// 	a.Log.Info(ctx, "push resources failed - agentFn", slog.Error(err))
	// 	return &proto.PushResourcesUsageResponse{}, err
	// }

	// workspace, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	// if err != nil {
	// 	a.Log.Info(ctx, "push resources failed - GetWorkspace", slog.Error(err))
	// 	return &proto.PushResourcesUsageResponse{}, err
	// }

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

	return &proto.PushResourcesUsageResponse{}, nil
}
