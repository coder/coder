package proto

import (
	"context"

	"storj.io/drpc"
)

// DRPCTailnetClient20 is the Tailnet API at v2.0.
type DRPCTailnetClient20 interface {
	DRPCConn() drpc.Conn

	StreamDERPMaps(ctx context.Context, in *StreamDERPMapsRequest) (DRPCTailnet_StreamDERPMapsClient, error)
	Coordinate(ctx context.Context) (DRPCTailnet_CoordinateClient, error)
}

// DRPCTailnetClient21 is the Tailnet API at v2.1. It is functionally identical to 2.0, because the
// change was to the Agent API (GetAnnouncementBanners).
type DRPCTailnetClient21 interface {
	DRPCTailnetClient20
}

// DRPCTailnetClient22 is the Tailnet API at v2.2. It adds telemetry support. Compatible with Coder
// v2.13+
type DRPCTailnetClient22 interface {
	DRPCTailnetClient21
	PostTelemetry(ctx context.Context, in *TelemetryRequest) (*TelemetryResponse, error)
}

// DRPCTailnetClient23 is the Tailnet API at v2.3. It adds resume token and workspace updates
// support. Compatible with Coder v2.18+.
type DRPCTailnetClient23 interface {
	DRPCTailnetClient22
	RefreshResumeToken(ctx context.Context, in *RefreshResumeTokenRequest) (*RefreshResumeTokenResponse, error)
	WorkspaceUpdates(ctx context.Context, in *WorkspaceUpdatesRequest) (DRPCTailnet_WorkspaceUpdatesClient, error)
}
