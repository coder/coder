package proto

import "github.com/coder/coder/v2/apiversion"

// Version history:
//
// API v1.2:
//   - Add support for `open_in` parameters in the workspace apps.
//
// API v1.3:
//   - Add new field named `resources_monitoring` in the Agent with resources monitoring.
//
// API v1.4:
//   - Add new field named `devcontainers` in the Agent.
//
// API v1.5:
//   - Add new field named `running_agent_auth_tokens` to provisioner job metadata
const (
	CurrentMajor = 1
	CurrentMinor = 5
)

// CurrentVersion is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
