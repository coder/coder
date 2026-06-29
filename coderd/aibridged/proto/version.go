package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.0:
//   - Initial version. Serves the Recorder, MCPConfigurator, and Authorizer
//     services to embedded and standalone AI Gateway daemons.
//
// API v1.1:
//   - Adds the ProviderConfigurator service with the GetAIProviders unary RPC,
//     letting embedded and standalone gateways fetch provider configuration
//     over DRPC instead of reading the database directly.
const (
	CurrentMajor = 1
	CurrentMinor = 1
)

// CurrentVersion is the current aibridged API version.
// Breaking changes to the aibridged API **MUST** increment CurrentMajor above.
// Non-breaking changes to the aibridged API **MUST** increment CurrentMinor
// above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
