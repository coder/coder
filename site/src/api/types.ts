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

export interface UserResponse {
  readonly id: string
  readonly username: string
  readonly email: string
  readonly created_at: string
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

/**
 * @remarks Keep in sync with codersdk/workspaces.go
 */
export interface Workspace {
  id: string
  created_at: string
  updated_at: string
  owner_id: string
  template_id: string
  name: string
  autostart_schedule: string
  autostop_schedule: string
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

export interface Pager {
  total: number
}

export interface PagedUsers {
  page: UserResponse[]
  pager: Pager
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
