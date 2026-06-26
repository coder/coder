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
//
// API v1.2:
//   - SyncList RPC (list all registered units)

const (
	CurrentMajor = 1
	CurrentMinor = 2
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
