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

func (a *ResourcesUsageAPI) PushResourcesUsage(ctx context.Context, req *proto.PushResourcesUsageRequest) (*proto.PushResourcesUsageResponse, error) {
	a.Log.Info(ctx, "push resources usage", slog.F("request", req))

	return &proto.PushResourcesUsageResponse{}, nil
}
