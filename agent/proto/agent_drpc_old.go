package proto

import (
	"context"

	"storj.io/drpc"
)

// DRPCAgentClient20 is the Agent API at v2.0.  Notably, it is missing GetAnnouncementBanners, but
// is useful when you want to be maximally compatible with Coderd Release Versions from 2.9+
type DRPCAgentClient20 interface {
	DRPCConn() drpc.Conn

	GetManifest(ctx context.Context, in *GetManifestRequest) (*Manifest, error)
	GetServiceBanner(ctx context.Context, in *GetServiceBannerRequest) (*ServiceBanner, error)
	UpdateStats(ctx context.Context, in *UpdateStatsRequest) (*UpdateStatsResponse, error)
	UpdateLifecycle(ctx context.Context, in *UpdateLifecycleRequest) (*Lifecycle, error)
	BatchUpdateAppHealths(ctx context.Context, in *BatchUpdateAppHealthRequest) (*BatchUpdateAppHealthResponse, error)
	UpdateStartup(ctx context.Context, in *UpdateStartupRequest) (*Startup, error)
	BatchUpdateMetadata(ctx context.Context, in *BatchUpdateMetadataRequest) (*BatchUpdateMetadataResponse, error)
	BatchCreateLogs(ctx context.Context, in *BatchCreateLogsRequest) (*BatchCreateLogsResponse, error)
}

// DRPCAgentClient21 is the Agent API at v2.1. It is useful if you want to be maximally compatible
// with Coderd Release Versions from 2.12+
type DRPCAgentClient21 interface {
	DRPCAgentClient20
	GetAnnouncementBanners(ctx context.Context, in *GetAnnouncementBannersRequest) (*GetAnnouncementBannersResponse, error)
}

// DRPCAgentClient22 is the Agent API at v2.2. It is identical to 2.1, since the change was made on
// the Tailnet API, which uses the same version number. Compatible with Coder v2.13+
type DRPCAgentClient22 interface {
	DRPCAgentClient21
}

// DRPCAgentClient23 is the Agent API at v2.3. It adds the ScriptCompleted RPC. Compatible with
// Coder v2.18+
type DRPCAgentClient23 interface {
	DRPCAgentClient22
	ScriptCompleted(ctx context.Context, in *WorkspaceAgentScriptCompletedRequest) (*WorkspaceAgentScriptCompletedResponse, error)
}
