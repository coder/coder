package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.0:
//   - Initial release
//   - Ping
//   - Sync operations: SyncStart, SyncWant, SyncComplete, SyncWait, SyncStatus
//
// API v1.1:
//   - UpdateAppStatus RPC (forwarded to coderd)

const (
	CurrentMajor = 1
	CurrentMinor = 1
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
