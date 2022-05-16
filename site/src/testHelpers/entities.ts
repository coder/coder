import * as Types from "../api/types"
import * as TypesGen from "../api/typesGenerated"

export const MockSessionToken: TypesGen.LoginWithPasswordResponse = {
  session_token: "my-session-token",
}

export const MockAPIKey: TypesGen.GenerateAPIKeyResponse = {
  key: "my-api-key",
}

export const MockBuildInfo: TypesGen.BuildInfoResponse = {
  external_url: "file:///mock-url",
  version: "v99.999.9999+c9cdf14",
}

export const MockAdminRole: TypesGen.Role = {
  name: "admin",
  display_name: "Admin",
}

export const MockMemberRole: TypesGen.Role = {
  name: "member",
  display_name: "Member",
}

export const MockAuditorRole: TypesGen.Role = {
  name: "auditor",
  display_name: "Auditor",
}

export const MockSiteRoles = [MockAdminRole, MockAuditorRole, MockMemberRole]

export const MockUser: TypesGen.User = {
  id: "test-user",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
  status: "active",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [MockAdminRole, MockMemberRole],
}

export const MockUser2: TypesGen.User = {
  id: "test-user-2",
  username: "TestUser2",
  email: "test2@coder.com",
  created_at: "",
  status: "active",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [MockMemberRole],
}

export const MockOrganization: TypesGen.Organization = {
  id: "test-org",
  name: "Test Organization",
  created_at: "",
  updated_at: "",
}

export const MockProvisioner: TypesGen.ProvisionerDaemon = {
  created_at: "",
  id: "test-provisioner",
  name: "Test Provisioner",
  provisioners: [],
}

export const MockProvisionerJob: TypesGen.ProvisionerJob = {
  created_at: "",
  id: "test-provisioner-job",
  status: "succeeded",
}

export const MockFailedProvisionerJob = { ...MockProvisionerJob, status: "failed" as TypesGen.ProvisionerJobStatus }
export const MockCancelingProvisionerJob = {
  ...MockProvisionerJob,
  status: "canceling" as TypesGen.ProvisionerJobStatus,
}
export const MockRunningProvisionerJob = { ...MockProvisionerJob, status: "running" as TypesGen.ProvisionerJobStatus }

export const MockTemplate: TypesGen.Template = {
  id: "test-template",
  created_at: "",
  updated_at: "",
  organization_id: MockOrganization.id,
  name: "Test Template",
  provisioner: MockProvisioner.id,
  active_version_id: "",
  workspace_owner_count: 1,
}

export const MockWorkspaceAutostartDisabled: TypesGen.UpdateWorkspaceAutostartRequest = {
  schedule: "",
}

export const MockWorkspaceAutostartEnabled: TypesGen.UpdateWorkspaceAutostartRequest = {
  // Runs at 9:30am Monday through Friday using Canada/Eastern
  // (America/Toronto) time
  schedule: "CRON_TZ=Canada/Eastern 30 9 * * 1-5",
}

export const MockWorkspaceAutostopDisabled: TypesGen.UpdateWorkspaceAutostartRequest = {
  schedule: "",
}

export const MockWorkspaceAutostopEnabled: TypesGen.UpdateWorkspaceAutostartRequest = {
  // Runs at 9:30pm Monday through Friday using America/Toronto
  schedule: "CRON_TZ=America/Toronto 30 21 * * 1-5",
}

export const MockWorkspaceBuild: TypesGen.WorkspaceBuild = {
  after_id: "",
  before_id: "",
  created_at: "",
  id: "test-workspace-build",
  initiator_id: "",
  job: MockProvisionerJob,
  name: "a-workspace-build",
  template_version_id: "",
  transition: "start",
  updated_at: "",
  workspace_id: "test-workspace",
}

export const MockWorkspaceBuildStop = {
  ...MockWorkspaceBuild,
  transition: "stop",
}

export const MockWorkspaceBuildDelete = {
  ...MockWorkspaceBuild,
  transition: "delete",
}

export const MockWorkspace: TypesGen.Workspace = {
  id: "test-workspace",
  name: "Test-Workspace",
  created_at: "",
  updated_at: "",
  template_id: MockTemplate.id,
  template_name: MockTemplate.name,
  outdated: false,
  owner_id: MockUser.id,
  owner_name: MockUser.username,
  autostart_schedule: MockWorkspaceAutostartEnabled.schedule,
  autostop_schedule: MockWorkspaceAutostopEnabled.schedule,
  latest_build: MockWorkspaceBuild,
}

export const MockStoppedWorkspace: TypesGen.Workspace = { ...MockWorkspace, latest_build: MockWorkspaceBuildStop }
export const MockStoppingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuildStop, job: MockRunningProvisionerJob },
}
export const MockStartingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuild, job: MockRunningProvisionerJob },
}
export const MockCancelingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuild, job: MockCancelingProvisionerJob },
}
export const MockFailedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuild, job: MockFailedProvisionerJob },
}
export const MockDeletingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuildDelete, job: MockRunningProvisionerJob },
}
export const MockDeletedWorkspace: TypesGen.Workspace = { ...MockWorkspace, latest_build: MockWorkspaceBuildDelete }

export const MockOutdatedWorkspace: TypesGen.Workspace = { ...MockWorkspace, outdated: true }

export const MockWorkspaceAgent: TypesGen.WorkspaceAgent = {
  architecture: "amd64",
  created_at: "",
  environment_variables: {},
  id: "test-workspace-agent",
  name: "a-workspace-agent",
  operating_system: "linux",
  resource_id: "",
  status: "connected",
  updated_at: "",
}

export const MockWorkspaceResource: TypesGen.WorkspaceResource = {
  agents: [MockWorkspaceAgent],
  created_at: "",
  id: "test-workspace-resource",
  job_id: "",
  name: "a-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
}

export const MockUserAgent: Types.UserAgent = {
  browser: "Chrome 99.0.4844",
  device: "Other",
  ip_address: "11.22.33.44",
  os: "Windows 10",
}

export const MockAuthMethods: TypesGen.AuthMethods = {
  password: true,
  github: false,
}

export const MockGitSSHKey: TypesGen.GitSSHKey = {
  user_id: "1fa0200f-7331-4524-a364-35770666caa7",
  created_at: "2022-05-16T14:30:34.148205897Z",
  updated_at: "2022-05-16T15:29:10.302441433Z",
  public_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFJOQRIM7kE30rOzrfy+/+R+nQGCk7S9pioihy+2ARbq",
}
