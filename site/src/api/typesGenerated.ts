// From codersdk/buildinfo.go:10:6.
export interface BuildInfoResponse {
  readonly external_url: string
  readonly version: string
}

// From codersdk/files.go:16:6.
export interface UploadResponse {
  readonly hash: string
}

// From codersdk/gitsshkey.go:14:6.
export interface GitSSHKey {
  readonly public_key: string
}

// From codersdk/gitsshkey.go:21:6.
export interface AgentGitSSHKey {
  readonly private_key: string
}

// From codersdk/organizations.go:17:6.
export interface Organization {
  readonly name: string
}

// From codersdk/organizations.go:25:6.
export interface CreateTemplateVersionRequest {
  readonly storage_source: string
}

// From codersdk/organizations.go:38:6.
export interface CreateTemplateRequest {
  readonly name: string
}

// From codersdk/parameters.go:26:6.
export interface Parameter {
  readonly scope: ParameterScope
  readonly name: string
}

// From codersdk/parameters.go:38:6.
export interface CreateParameterRequest {
  readonly name: string
  readonly source_value: string
}

// From codersdk/provisionerdaemons.go:37:6.
export interface ProvisionerJob {
  readonly error: string
  readonly status: ProvisionerJobStatus
}

// From codersdk/provisionerdaemons.go:47:6.
export interface ProvisionerJobLog {
  readonly stage: string
  readonly output: string
}

// From codersdk/templates.go:17:6.
export interface Template {
  readonly name: string
  readonly workspace_owner_count: number
}

// From codersdk/templateversions.go:17:6.
export interface TemplateVersion {
  readonly name: string
  readonly job: ProvisionerJob
}

// From codersdk/users.go:17:6.
export interface UsersRequest {
  readonly search: string
  readonly limit: number
  readonly offset: number
}

// From codersdk/users.go:32:6.
export interface User {
  readonly email: string
  readonly username: string
  readonly name: string
}

// From codersdk/users.go:40:6.
export interface CreateFirstUserRequest {
  readonly email: string
  readonly username: string
  readonly password: string
  readonly organization: string
}

// From codersdk/users.go:53:6.
export interface CreateUserRequest {
  readonly email: string
  readonly username: string
  readonly password: string
}

// From codersdk/users.go:60:6.
export interface UpdateUserProfileRequest {
  readonly email: string
  readonly username: string
  readonly name?: string
}

// From codersdk/users.go:67:6.
export interface LoginWithPasswordRequest {
  readonly email: string
  readonly password: string
}

// From codersdk/users.go:73:6.
export interface LoginWithPasswordResponse {
  readonly session_token: string
}

// From codersdk/users.go:78:6.
export interface GenerateAPIKeyResponse {
  readonly key: string
}

// From codersdk/users.go:82:6.
export interface CreateOrganizationRequest {
  readonly name: string
}

// From codersdk/users.go:87:6.
export interface CreateWorkspaceRequest {
  readonly name: string
}

// From codersdk/workspaceagents.go:31:6.
export interface GoogleInstanceIdentityToken {
  readonly json_web_token: string
}

// From codersdk/workspaceagents.go:35:6.
export interface AWSInstanceIdentityToken {
  readonly signature: string
  readonly document: string
}

// From codersdk/workspaceagents.go:40:6.
export interface AzureInstanceIdentityToken {
  readonly signature: string
  readonly encoding: string
}

// From codersdk/workspaceagents.go:47:6.
export interface WorkspaceAgentAuthenticateResponse {
  readonly session_token: string
}

// From codersdk/workspacebuilds.go:17:6.
export interface WorkspaceBuild {
  readonly name: string
  readonly job: ProvisionerJob
}

// From codersdk/workspaceresources.go:23:6.
export interface WorkspaceResource {
  readonly type: string
  readonly name: string
}

// From codersdk/workspaceresources.go:33:6.
export interface WorkspaceAgent {
  readonly status: WorkspaceAgentStatus
  readonly name: string
  readonly instance_id: string
  readonly architecture: string
  readonly operating_system: string
  readonly startup_script: string
}

// From codersdk/workspaceresources.go:50:6.
export interface WorkspaceAgentResourceMetadata {
  readonly memory_total: number
  readonly disk_total: number
  readonly cpu_cores: number
  readonly cpu_model: string
  readonly cpu_mhz: number
}

// From codersdk/workspaceresources.go:58:6.
export interface WorkspaceAgentInstanceMetadata {
  readonly jail_orchestrator: string
  readonly operating_system: string
  readonly platform: string
  readonly platform_family: string
  readonly kernel_version: string
  readonly kernel_architecture: string
  readonly cloud: string
  readonly jail: string
  readonly vnc: boolean
}

// From codersdk/workspaces.go:18:6.
export interface Workspace {
  readonly template_name: string
  readonly latest_build: WorkspaceBuild
  readonly outdated: boolean
  readonly name: string
  readonly autostart_schedule: string
  readonly autostop_schedule: string
}

// From codersdk/workspaces.go:33:6.
export interface CreateWorkspaceBuildRequest {
  readonly dry_run: boolean
}

// From codersdk/workspaces.go:94:6.
export interface UpdateWorkspaceAutostartRequest {
  readonly schedule: string
}

// From codersdk/workspaces.go:114:6.
export interface UpdateWorkspaceAutostopRequest {
  readonly schedule: string
}

// From codersdk/parameters.go:16:6.
export type ParameterScope = "organization" | "template" | "user" | "workspace"

// From codersdk/provisionerdaemons.go:26:6.
export type ProvisionerJobStatus = "pending" | "running" | "succeeded" | "canceling" | "canceled" | "failed"

// From codersdk/workspaceresources.go:15:6.
export type WorkspaceAgentStatus = "connecting" | "connected" | "disconnected"
