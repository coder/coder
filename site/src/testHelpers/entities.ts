import {
  BuildInfoResponse,
  Organization,
  Pager,
  Provisioner,
  Template,
  UserAgent,
  UserResponse,
  Workspace,
  WorkspaceAutostartRequest,
} from "../api/types"
import { AuthMethods } from "../api/typesGenerated"

export const MockSessionToken = { session_token: "my-session-token" }

export const MockAPIKey = { key: "my-api-key" }

export const MockBuildInfo: BuildInfoResponse = {
  external_url: "file:///mock-url",
  version: "v99.999.9999+c9cdf14",
}

export const MockUser: UserResponse = {
  name: "Test User",
  id: "test-user",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
}

export const MockUser2: UserResponse = {
  id: "test-user-2",
  name: "Test User 2",
  username: "TestUser2",
  email: "test2@coder.com",
  created_at: "",
}

export const MockPager: Pager = {
  total: 2,
}

export const MockOrganization: Organization = {
  id: "test-org",
  name: "Test Organization",
  created_at: "",
  updated_at: "",
}

export const MockProvisioner: Provisioner = {
  id: "test-provisioner",
  name: "Test Provisioner",
}

export const MockTemplate: Template = {
  id: "test-template",
  created_at: "",
  updated_at: "",
  organization_id: MockOrganization.id,
  name: "Test Template",
  provisioner: MockProvisioner.id,
  active_version_id: "",
}

export const MockWorkspaceAutostartDisabled: WorkspaceAutostartRequest = {
  schedule: "",
}

export const MockWorkspaceAutostartEnabled: WorkspaceAutostartRequest = {
  // Runs at 9:30am Monday through Friday using Canada/Eastern
  // (America/Toronto) time
  schedule: "CRON_TZ=Canada/Eastern 30 9 * * 1-5",
}

export const MockWorkspaceAutostopDisabled: WorkspaceAutostartRequest = {
  schedule: "",
}

export const MockWorkspaceAutostopEnabled: WorkspaceAutostartRequest = {
  // Runs at 9:30pm Monday through Friday using America/Toronto
  schedule: "CRON_TZ=America/Toronto 30 21 * * 1-5",
}

export const MockWorkspace: Workspace = {
  id: "test-workspace",
  name: "Test-Workspace",
  created_at: "",
  updated_at: "",
  template_id: MockTemplate.id,
  owner_id: MockUser.id,
  autostart_schedule: MockWorkspaceAutostartEnabled.schedule,
  autostop_schedule: MockWorkspaceAutostopEnabled.schedule,
}

export const MockUserAgent: UserAgent = {
  browser: "Chrome 99.0.4844",
  device: "Other",
  ip_address: "11.22.33.44",
  os: "Windows 10",
}

export const MockAuthMethods: AuthMethods = {
  password: true,
  github: false,
}
