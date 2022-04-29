/**
 * `BuildInfoResponse` must be kept in sync with the go struct in buildinfo.go.
 */
export interface BuildInfoResponse {
  external_url: string
  version: string
}

export interface LoginResponse {
  session_token: string
}

export interface CreateUserRequest {
  username: string
  email: string
  password: string
  organization_id: string
}

export interface UserResponse {
  readonly id: string
  readonly username: string
  readonly email: string
  readonly created_at: string
  readonly status: "active" | "suspended"
  readonly organization_ids: string[]
}

/**
 * `Organization` must be kept in sync with the go struct in organizations.go
 */
export interface Organization {
  id: string
  name: string
  created_at: string
  updated_at: string
}

export interface Provisioner {
  id: string
  name: string
}

// This must be kept in sync with the `Template` struct in the back-end
export interface Template {
  id: string
  created_at: string
  updated_at: string
  organization_id: string
  name: string
  provisioner: string
  active_version_id: string
}

export interface CreateTemplateRequest {
  name: string
  organizationId: string
  provisioner: string
}

export interface CreateWorkspaceRequest {
  name: string
  template_id: string
}

export interface WorkspaceBuild {
  id: string
}

export interface Workspace {
  id: string
  created_at: string
  updated_at: string
  owner_id: string
  template_id: string
  name: string
  autostart_schedule: string
  autostop_schedule: string
  latest_build: WorkspaceBuild
}

export interface WorkspaceResource {
  id: string
  agents?: WorkspaceAgent[]
}

export interface WorkspaceAgent {
  id: string
  name: string
  operating_system: string
}

export interface APIKeyResponse {
  key: string
}

export interface UserAgent {
  readonly browser: string
  readonly device: string
  readonly ip_address: string
  readonly os: string
}

export interface WorkspaceAutostartRequest {
  schedule: string
}

export interface WorkspaceAutostopRequest {
  schedule: string
}

export interface UpdateProfileRequest {
  readonly username: string
  readonly email: string
}

export interface ReconnectingPTYRequest {
  readonly data?: string
  readonly height?: number
  readonly width?: number
}
