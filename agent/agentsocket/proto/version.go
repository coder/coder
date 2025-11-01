package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.0:
//   - Initial release
//   - Ping
//   - Sync operations: SyncStart, SyncWant, SyncComplete, SyncWait, SyncStatus

const (
	CurrentMajor = 1
	CurrentMinor = 0
)

var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
