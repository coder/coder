package proto

import (
	"github.com/coder/coder/v2/apiversion"
)

// Version history:
//
// API v1:
//   - retroactively applied name for the HTTP Rest APIs for the Agent and the
//     JSON over websocket coordination and DERP Map APIs for Tailnet
//
// API v2.0:
//   - Shipped in Coder v2.8.0
//   - first dRPC over yamux over websocket APIs for tailnet and agent
//
// API v2.1:
//   - Shipped in Coder v2.12.0
//   - Added support for multiple banners via the GetAnnouncementBanners RPC on
//     the Agent API.
//   - No changes to the Tailnet API.
//
// API v2.2:
//   - Shipped in Coder v2.13.0
//   - Added support for network telemetry via the PostTelemetry RPC on the
//     Tailnet API.
//   - No changes to the Agent API.
//
// API v2.3:
//   - Shipped in Coder v2.18.0
//   - Added support for client Resume Tokens on the Tailnet API via the
//     RefreshResumeToken RPC. (This actually shipped in Coder v2.15.0, but we
//     forgot to increment the API version. If you dial for API v2.2, you MAY
//     be connected to a server that supports RefreshResumeToken, but be
//     prepared to process "unsupported" errors.)
//   - Added support for WorkspaceUpdates RPC on the Tailnet API.
//   - Added support for ScriptCompleted RPC on the Agent API. (This actually
//     shipped in Coder v2.16.0, but we forgot to increment the API version. If
//     you dial for API v2.2, you MAY be connected to a server that supports
//     ScriptCompleted, but be prepared to process "unsupported" errors.)
//
// API v2.4:
//   - Shipped in Coder v2.{{placeholder}} // TODO Vincent: Replace with the correct version
//   - Added support for GetResourcesMonitoringConfiguration and
//     PushResourcesMonitoringUsage RPCs on the Agent API.
//   - Added support for reporting connection events for auditing via the
//     ReportConnection RPC on the Agent API.
const (
	CurrentMajor = 2
	CurrentMinor = 5
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
