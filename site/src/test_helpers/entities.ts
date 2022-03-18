import { Provisioner, Organization, Project, Workspace, UserResponse } from "../api/types"

export const MockSessionToken = { session_token: "my-session-token" }

export const MockAPIKey = { key: "my-api-key" }

export const MockUser: UserResponse = {
  id: "test-user-id",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
}

export const MockProject: Project = {
  id: "project-id",
  created_at: "",
  updated_at: "",
  organization_id: "test-org",
  name: "Test Project",
  provisioner: "test-provisioner",
  active_version_id: "",
}

export const MockProvisioner: Provisioner = {
  id: "test-provisioner",
  name: "Test Provisioner",
}

export const MockOrganization: Organization = {
  id: "test-org",
  name: "Test Organization",
  created_at: "",
  updated_at: "",
}

export const MockWorkspace: Workspace = {
  id: "test-workspace",
  name: "Test-Workspace",
  created_at: "",
  updated_at: "",
  project_id: "project-id",
  owner_id: "test-user-id",
}
