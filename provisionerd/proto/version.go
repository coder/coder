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
//   - Add `plan` and `module_files` fields to `CompletedJob.TemplateImport`.
const (
	CurrentMajor = 1
	CurrentMinor = 5
)

// CurrentVersion is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
