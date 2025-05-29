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
//   - Add new field named `prebuilt_workspace_build_stage` enum in the Metadata message.
//   - Add new field named `running_agent_auth_tokens` to provisioner job metadata
//   - Add new field named `resource_replacements` in PlanComplete & CompletedJob.WorkspaceBuild.
//   - Add new field named `api_key_scope` to WorkspaceAgent to support running without user data access.
//   - Add `plan` field to `CompletedJob.TemplateImport`.
//
// API v1.6:
//   - Add `module_files` field to `CompletedJob.TemplateImport`.
//   - Add previous parameter values to 'WorkspaceBuild' jobs. Provisioner passes
//     the previous values for the `terraform apply` to enforce monotonicity
//     in the terraform provider.
//   - Add new field named `expiration_policy` to `Prebuild`, with a field named
//     `ttl` to define TTL-based expiration for unclaimed prebuilds.
//   - Add `group` field to `App`
//   - Add `form_type` field to parameters
const (
	CurrentMajor = 1
	CurrentMinor = 6
)

// CurrentVersion is the current provisionerd API version.
// Breaking changes to the provisionerd API **MUST** increment
// CurrentMajor above.
// Non-breaking changes to the provisionerd API **MUST** increment
// CurrentMinor above.
var CurrentVersion = apiversion.New(CurrentMajor, CurrentMinor)
