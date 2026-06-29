package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.0:
//   - Initial version. Serves the Recorder, MCPConfigurator, and Authorizer
//     services to embedded and standalone AI Gateway daemons.
const (
	CurrentMajor = 1
	CurrentMinor = 0
)

// CurrentVersion is the current aibridged API version.
// Breaking changes to the aibridged API **MUST** increment CurrentMajor above.
// Non-breaking changes to the aibridged API **MUST** increment CurrentMinor
// above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
