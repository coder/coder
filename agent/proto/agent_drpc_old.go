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
	DRPCConn() drpc.Conn

	GetManifest(ctx context.Context, in *GetManifestRequest) (*Manifest, error)
	GetServiceBanner(ctx context.Context, in *GetServiceBannerRequest) (*ServiceBanner, error)
	UpdateStats(ctx context.Context, in *UpdateStatsRequest) (*UpdateStatsResponse, error)
	UpdateLifecycle(ctx context.Context, in *UpdateLifecycleRequest) (*Lifecycle, error)
	BatchUpdateAppHealths(ctx context.Context, in *BatchUpdateAppHealthRequest) (*BatchUpdateAppHealthResponse, error)
	UpdateStartup(ctx context.Context, in *UpdateStartupRequest) (*Startup, error)
	BatchUpdateMetadata(ctx context.Context, in *BatchUpdateMetadataRequest) (*BatchUpdateMetadataResponse, error)
	BatchCreateLogs(ctx context.Context, in *BatchCreateLogsRequest) (*BatchCreateLogsResponse, error)
	GetAnnouncementBanners(ctx context.Context, in *GetAnnouncementBannersRequest) (*GetAnnouncementBannersResponse, error)
}
