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
//
// API v1.3:
//   - Removed the UpdateAppStatus RPC along with workspace app statuses. Calls
//     now fail with an unknown RPC error.

const (
	CurrentMajor = 1
	CurrentMinor = 3
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
