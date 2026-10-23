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
// API v2.0:
//   - Removed the UpdateAppStatus RPC along with workspace app statuses.
//   - Not backward compatible: v1.x clients that call UpdateAppStatus get an
//     unknown RPC error.

const (
	CurrentMajor = 2
	CurrentMinor = 0
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
