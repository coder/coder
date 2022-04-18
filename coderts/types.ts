export interface BuildInfoResponse {
  external_url: string
  version: string
}

export interface UploadResponse {
  hash: string
}

export interface GitSSHKey {
  public_key: string
}

export interface AgentGitSSHKey {
  private_key: string
}

export interface Organization {
  name: string
}

export interface CreateTemplateVersionRequest {
  storage_source: string
}

export interface CreateTemplateRequest {
  name: string
}

type ParameterScope = string

const ParameterOrganization: ParameterScope = "organization"

const ParameterTemplate: ParameterScope = "template"

const ParameterUser: ParameterScope = "user"

const ParameterWorkspace: ParameterScope = "workspace"

export interface Parameter {
  scope: ParameterScope
  name: string
}

export interface CreateParameterRequest {
  name: string
  source_value: string
}

type ProvisionerJobStatus = string

const ProvisionerJobPending: ProvisionerJobStatus = "pending"

const ProvisionerJobRunning: ProvisionerJobStatus = "running"

const ProvisionerJobSucceeded: ProvisionerJobStatus = "succeeded"

const ProvisionerJobCanceling: ProvisionerJobStatus = "canceling"

const ProvisionerJobCanceled: ProvisionerJobStatus = "canceled"

const ProvisionerJobFailed: ProvisionerJobStatus = "failed"

export interface ProvisionerJob {
  error: string
  status: ProvisionerJobStatus
}

export interface ProvisionerJobLog {
  stage: string
  output: string
}

export interface Template {
  name: string
  workspace_owner_count: number
}

export interface TemplateVersion {
  name: string
  job: ProvisionerJob
}

export interface User {
  email: string
  username: string
  name: string
}

export interface CreateFirstUserRequest {
  email: string
  username: string
  password: string
  organization: string
}

export interface CreateUserRequest {
  email: string
  username: string
  password: string
}

export interface UpdateUserProfileRequest {
  email: string
  username: string
  name?: string
}

export interface LoginWithPasswordRequest {
  email: string
  password: string
}

export interface LoginWithPasswordResponse {
  session_token: string
}

export interface GenerateAPIKeyResponse {
  key: string
}

export interface CreateOrganizationRequest {
  name: string
}

export interface CreateWorkspaceRequest {
  name: string
}

export interface GoogleInstanceIdentityToken {
  json_web_token: string
}

export interface AWSInstanceIdentityToken {
  signature: string
  document: string
}

export interface WorkspaceAgentAuthenticateResponse {
  session_token: string
}

export interface WorkspaceBuild {
  name: string
  job: ProvisionerJob
}

type WorkspaceAgentStatus = string

const WorkspaceAgentConnecting: WorkspaceAgentStatus = "connecting"

const WorkspaceAgentConnected: WorkspaceAgentStatus = "connected"

const WorkspaceAgentDisconnected: WorkspaceAgentStatus = "disconnected"

export interface WorkspaceResource {
  type: string
  name: string
}

export interface WorkspaceAgent {
  status: WorkspaceAgentStatus
  name: string
  instance_id: string
  architecture: string
  operating_system: string
  startup_script: string
}

export interface WorkspaceAgentResourceMetadata {
  memory_total: number
  disk_total: number
  cpu_cores: number
  cpu_model: string
  cpu_mhz: number
}

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

export interface Workspace {
  template_name: string
  latest_build: WorkspaceBuild
  outdated: boolean
  name: string
  autostart_schedule: string
  autostop_schedule: string
}

export interface CreateWorkspaceBuildRequest {
  dry_run: boolean
}

export interface UpdateWorkspaceAutostartRequest {
  schedule: string
}

export interface UpdateWorkspaceAutostopRequest {
  schedule: string
}

