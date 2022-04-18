// From codersdk/buildinfo.go:10:6.
export interface BuildInfoResponse {
  external_url: string
  version: string
}

// From codersdk/files.go:16:6.
export interface UploadResponse {
  hash: string
}

// From codersdk/gitsshkey.go:14:6.
export interface GitSSHKey {
  public_key: string
}

// From codersdk/gitsshkey.go:21:6.
export interface AgentGitSSHKey {
  private_key: string
}

// From codersdk/organizations.go:17:6.
export interface Organization {
  name: string
}

// From codersdk/organizations.go:25:6.
export interface CreateTemplateVersionRequest {
  storage_source: string
}

// From codersdk/organizations.go:38:6.
export interface CreateTemplateRequest {
  name: string
}

// From codersdk/parameters.go:16:6.
type ParameterScope = string

// From codersdk/parameters.go:19:2.
const ParameterOrganization: ParameterScope = "organization"

// From codersdk/parameters.go:20:2.
const ParameterTemplate: ParameterScope = "template"

// From codersdk/parameters.go:21:2.
const ParameterUser: ParameterScope = "user"

// From codersdk/parameters.go:22:2.
const ParameterWorkspace: ParameterScope = "workspace"

// From codersdk/parameters.go:26:6.
export interface Parameter {
  scope: ParameterScope
  name: string
}

// From codersdk/parameters.go:38:6.
export interface CreateParameterRequest {
  name: string
  source_value: string
}

// From codersdk/provisionerdaemons.go:26:6.
type ProvisionerJobStatus = string

// From codersdk/provisionerdaemons.go:29:2.
const ProvisionerJobPending: ProvisionerJobStatus = "pending"

// From codersdk/provisionerdaemons.go:30:2.
const ProvisionerJobRunning: ProvisionerJobStatus = "running"

// From codersdk/provisionerdaemons.go:31:2.
const ProvisionerJobSucceeded: ProvisionerJobStatus = "succeeded"

// From codersdk/provisionerdaemons.go:32:2.
const ProvisionerJobCanceling: ProvisionerJobStatus = "canceling"

// From codersdk/provisionerdaemons.go:33:2.
const ProvisionerJobCanceled: ProvisionerJobStatus = "canceled"

// From codersdk/provisionerdaemons.go:34:2.
const ProvisionerJobFailed: ProvisionerJobStatus = "failed"

// From codersdk/provisionerdaemons.go:37:6.
export interface ProvisionerJob {
  error: string
  status: ProvisionerJobStatus
}

// From codersdk/provisionerdaemons.go:47:6.
export interface ProvisionerJobLog {
  stage: string
  output: string
}

// From codersdk/templates.go:17:6.
export interface Template {
  name: string
  workspace_owner_count: number
}

// From codersdk/templateversions.go:17:6.
export interface TemplateVersion {
  name: string
  job: ProvisionerJob
}

// From codersdk/users.go:17:6.
export interface User {
  email: string
  username: string
  name: string
}

// From codersdk/users.go:25:6.
export interface CreateFirstUserRequest {
  email: string
  username: string
  password: string
  organization: string
}

// From codersdk/users.go:38:6.
export interface CreateUserRequest {
  email: string
  username: string
  password: string
}

// From codersdk/users.go:45:6.
export interface UpdateUserProfileRequest {
  email: string
  username: string
  name?: string
}

// From codersdk/users.go:52:6.
export interface LoginWithPasswordRequest {
  email: string
  password: string
}

// From codersdk/users.go:58:6.
export interface LoginWithPasswordResponse {
  session_token: string
}

// From codersdk/users.go:63:6.
export interface GenerateAPIKeyResponse {
  key: string
}

// From codersdk/users.go:67:6.
export interface CreateOrganizationRequest {
  name: string
}

// From codersdk/users.go:72:6.
export interface CreateWorkspaceRequest {
  name: string
}

// From codersdk/workspaceagents.go:26:6.
export interface GoogleInstanceIdentityToken {
  json_web_token: string
}

// From codersdk/workspaceagents.go:30:6.
export interface AWSInstanceIdentityToken {
  signature: string
  document: string
}

// From codersdk/workspaceagents.go:37:6.
export interface WorkspaceAgentAuthenticateResponse {
  session_token: string
}

// From codersdk/workspacebuilds.go:17:6.
export interface WorkspaceBuild {
  name: string
  job: ProvisionerJob
}

// From codersdk/workspaceresources.go:15:6.
type WorkspaceAgentStatus = string

// From codersdk/workspaceresources.go:18:2.
const WorkspaceAgentConnecting: WorkspaceAgentStatus = "connecting"

// From codersdk/workspaceresources.go:19:2.
const WorkspaceAgentConnected: WorkspaceAgentStatus = "connected"

// From codersdk/workspaceresources.go:20:2.
const WorkspaceAgentDisconnected: WorkspaceAgentStatus = "disconnected"

// From codersdk/workspaceresources.go:23:6.
export interface WorkspaceResource {
  type: string
  name: string
}

// From codersdk/workspaceresources.go:33:6.
export interface WorkspaceAgent {
  status: WorkspaceAgentStatus
  name: string
  instance_id: string
  architecture: string
  operating_system: string
  startup_script: string
}

// From codersdk/workspaceresources.go:50:6.
export interface WorkspaceAgentResourceMetadata {
  memory_total: number
  disk_total: number
  cpu_cores: number
  cpu_model: string
  cpu_mhz: number
}

// From codersdk/workspaceresources.go:58:6.
export interface WorkspaceAgentInstanceMetadata {
  jail_orchestrator: string
  operating_system: string
  platform: string
  platform_family: string
  kernel_version: string
  kernel_architecture: string
  cloud: string
  jail: string
  vnc: boolean
}

// From codersdk/workspaces.go:18:6.
export interface Workspace {
  template_name: string
  latest_build: WorkspaceBuild
  outdated: boolean
  name: string
  autostart_schedule: string
  autostop_schedule: string
}

// From codersdk/workspaces.go:33:6.
export interface CreateWorkspaceBuildRequest {
  dry_run: boolean
}

// From codersdk/workspaces.go:94:6.
export interface UpdateWorkspaceAutostartRequest {
  schedule: string
}

// From codersdk/workspaces.go:114:6.
export interface UpdateWorkspaceAutostopRequest {
  schedule: string
}

