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

// This must be kept in sync with the `Project` struct in the back-end
export interface Project {
  id: string
  created_at: string
  updated_at: string
  organization_id: string
  name: string
  provisioner: string
  active_version_id: string
}

export interface CreateProjectRequest {
  name: string
  organizationId: string
  provisioner: string
}

export interface CreateWorkspaceRequest {
  name: string
  project_id: string
}

// Must be kept in sync with backend Workspace struct
export interface Workspace {
  id: string
  created_at: string
  updated_at: string
  owner_id: string
  project_id: string
  name: string
}

export interface APIKeyResponse {
  key: string
}
