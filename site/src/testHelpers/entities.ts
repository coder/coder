import { FieldError } from "api/errors"
import { everyOneGroup } from "util/groups"
import * as Types from "../api/types"
import * as TypesGen from "../api/typesGenerated"

export const MockTemplateDAUResponse: TypesGen.TemplateDAUsResponse = {
  entries: [
    { date: "2022-08-27T00:00:00Z", amount: 1 },
    { date: "2022-08-29T00:00:00Z", amount: 2 },
    { date: "2022-08-30T00:00:00Z", amount: 1 },
  ],
}
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

export const MockOwnerRole: TypesGen.Role = {
  name: "owner",
  display_name: "Owner",
}

export const MockUserAdminRole: TypesGen.Role = {
  name: "user_admin",
  display_name: "User Admin",
}

export const MockTemplateAdminRole: TypesGen.Role = {
  name: "template_admin",
  display_name: "Template Admin",
}

export const MockMemberRole: TypesGen.Role = {
  name: "member",
  display_name: "Member",
}

export const MockAuditorRole: TypesGen.Role = {
  name: "auditor",
  display_name: "Auditor",
}

// assignableRole takes a role and a boolean. The boolean implies if the
// actor can assign (add/remove) the role from other users.
export function assignableRole(
  role: TypesGen.Role,
  assignable: boolean,
): TypesGen.AssignableRoles {
  return {
    ...role,
    assignable: assignable,
  }
}

export const MockSiteRoles = [MockUserAdminRole, MockAuditorRole]
export const MockAssignableSiteRoles = [
  assignableRole(MockUserAdminRole, true),
  assignableRole(MockAuditorRole, true),
]

export const MockMemberPermissions = {
  viewAuditLog: false,
}

export const MockUser: TypesGen.User = {
  id: "test-user",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
  status: "active",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [MockOwnerRole],
  avatar_url: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
  last_seen_at: "",
}

export const MockUserAdmin: TypesGen.User = {
  id: "test-user",
  username: "TestUser",
  email: "test@coder.com",
  created_at: "",
  status: "active",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [MockUserAdminRole],
  avatar_url: "",
  last_seen_at: "",
}

export const MockUser2: TypesGen.User = {
  id: "test-user-2",
  username: "TestUser2",
  email: "test2@coder.com",
  created_at: "",
  status: "active",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [],
  avatar_url: "",
  last_seen_at: "2022-09-14T19:12:21Z",
}

export const SuspendedMockUser: TypesGen.User = {
  id: "suspended-mock-user",
  username: "SuspendedMockUser",
  email: "iamsuspendedsad!@coder.com",
  created_at: "",
  status: "suspended",
  organization_ids: ["fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0"],
  roles: [],
  avatar_url: "",
  last_seen_at: "",
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
  provisioners: ["echo"],
}

export const MockProvisionerJob: TypesGen.ProvisionerJob = {
  created_at: "",
  id: "test-provisioner-job",
  status: "succeeded",
  file_id: "fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0",
  completed_at: "2022-05-17T17:39:01.382927298Z",
}

export const MockFailedProvisionerJob: TypesGen.ProvisionerJob = {
  ...MockProvisionerJob,
  status: "failed",
}

export const MockCancelingProvisionerJob: TypesGen.ProvisionerJob = {
  ...MockProvisionerJob,
  status: "canceling",
}
export const MockCanceledProvisionerJob: TypesGen.ProvisionerJob = {
  ...MockProvisionerJob,
  status: "canceled",
}
export const MockRunningProvisionerJob: TypesGen.ProvisionerJob = {
  ...MockProvisionerJob,
  status: "running",
}
export const MockPendingProvisionerJob: TypesGen.ProvisionerJob = {
  ...MockProvisionerJob,
  status: "pending",
}
export const MockTemplateVersion: TypesGen.TemplateVersion = {
  id: "test-template-version",
  created_at: "2022-05-17T17:39:01.382927298Z",
  updated_at: "2022-05-17T17:39:01.382927298Z",
  template_id: "test-template",
  job: MockProvisionerJob,
  name: "test-version",
  readme: `---
name:Template test
---
## Instructions
You can add instructions here

[Some link info](https://coder.com)`,
  created_by: MockUser,
}

export const MockTemplate: TypesGen.Template = {
  id: "test-template",
  created_at: "2022-05-17T17:39:01.382927298Z",
  updated_at: "2022-05-18T17:39:01.382927298Z",
  organization_id: MockOrganization.id,
  name: "test-template",
  provisioner: MockProvisioner.provisioners[0],
  active_version_id: MockTemplateVersion.id,
  workspace_owner_count: 2,
  active_user_count: 1,
  build_time_stats: {
    start_ms: 1000,
    stop_ms: 2000,
    delete_ms: 3000,
  },
  description: "This is a test description.",
  max_ttl_ms: 24 * 60 * 60 * 1000,
  min_autostart_interval_ms: 60 * 60 * 1000,
  created_by_id: "test-creator-id",
  created_by_name: "test_creator",
  icon: "/icon/code.svg",
}

export const MockWorkspaceApp: TypesGen.WorkspaceApp = {
  id: "test-app",
  slug: "test-app",
  display_name: "Test App",
  icon: "",
  subdomain: false,
  health: "disabled",
  sharing_level: "owner",
  healthcheck: {
    url: "",
    interval: 0,
    threshold: 0,
  },
}

export const MockWorkspaceAgent: TypesGen.WorkspaceAgent = {
  apps: [MockWorkspaceApp],
  architecture: "amd64",
  created_at: "",
  environment_variables: {},
  id: "test-workspace-agent",
  name: "a-workspace-agent",
  operating_system: "linux",
  resource_id: "",
  status: "connected",
  updated_at: "",
  version: MockBuildInfo.version,
  latency: {
    "Coder Embedded DERP": {
      latency_ms: 32.55,
      preferred: true,
    },
  },
}

export const MockWorkspaceAgentDisconnected: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-2",
  name: "another-workspace-agent",
  status: "disconnected",
  version: "",
  latency: {},
}

export const MockWorkspaceAgentOutdated: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-3",
  name: "an-outdated-workspace-agent",
  version: "v99.999.9998+abcdef",
  operating_system: "Windows",
  latency: {
    ...MockWorkspaceAgent.latency,
    Chicago: {
      preferred: false,
      latency_ms: 95.11,
    },
    "San Francisco": {
      preferred: false,
      latency_ms: 111.55,
    },
    Paris: {
      preferred: false,
      latency_ms: 221.66,
    },
  },
}

export const MockWorkspaceAgentConnecting: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-connecting",
  name: "another-workspace-agent",
  status: "connecting",
  version: "",
  latency: {},
}

export const MockWorkspaceResource: TypesGen.WorkspaceResource = {
  agents: [
    MockWorkspaceAgent,
    MockWorkspaceAgentConnecting,
    MockWorkspaceAgentOutdated,
  ],
  created_at: "",
  id: "test-workspace-resource",
  job_id: "",
  name: "a-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
  hide: false,
  icon: "",
  metadata: [
    { key: "type", value: "a-workspace-resource", sensitive: false },
    { key: "api_key", value: "12345678", sensitive: true },
  ],
}

export const MockWorkspaceResource2: TypesGen.WorkspaceResource = {
  agents: [
    MockWorkspaceAgent,
    MockWorkspaceAgentDisconnected,
    MockWorkspaceAgentOutdated,
  ],
  created_at: "",
  id: "test-workspace-resource-2",
  job_id: "",
  name: "another-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
  hide: false,
  icon: "",
  metadata: [
    { key: "type", value: "google_compute_disk", sensitive: false },
    { key: "size", value: "32GB", sensitive: false },
  ],
}

export const MockWorkspaceResource3: TypesGen.WorkspaceResource = {
  agents: [
    MockWorkspaceAgent,
    MockWorkspaceAgentDisconnected,
    MockWorkspaceAgentOutdated,
  ],
  created_at: "",
  id: "test-workspace-resource-3",
  job_id: "",
  name: "another-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
  hide: true,
  icon: "",
  metadata: [
    { key: "type", value: "google_compute_disk", sensitive: false },
    { key: "size", value: "32GB", sensitive: false },
  ],
}

export const MockWorkspaceAutostartDisabled: TypesGen.UpdateWorkspaceAutostartRequest =
  {
    schedule: "",
  }

export const MockWorkspaceAutostartEnabled: TypesGen.UpdateWorkspaceAutostartRequest =
  {
    // Runs at 9:30am Monday through Friday using Canada/Eastern
    // (America/Toronto) time
    schedule: "CRON_TZ=Canada/Eastern 30 9 * * 1-5",
  }

export const MockWorkspaceBuild: TypesGen.WorkspaceBuild = {
  build_number: 1,
  created_at: "2022-05-17T17:39:01.382927298Z",
  id: "1",
  initiator_id: MockUser.id,
  initiator_name: MockUser.username,
  job: MockProvisionerJob,
  template_version_id: "",
  transition: "start",
  updated_at: "2022-05-17T17:39:01.382927298Z",
  workspace_name: "test-workspace",
  workspace_owner_id: MockUser.id,
  workspace_owner_name: MockUser.username,
  workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
  deadline: "2022-05-17T23:39:00.00Z",
  reason: "initiator",
  resources: [MockWorkspaceResource],
  status: "running",
}

export const MockFailedWorkspaceBuild = (
  transition: TypesGen.WorkspaceTransition = "start",
): TypesGen.WorkspaceBuild => ({
  build_number: 1,
  created_at: "2022-05-17T17:39:01.382927298Z",
  id: "1",
  initiator_id: MockUser.id,
  initiator_name: MockUser.username,
  job: MockFailedProvisionerJob,
  template_version_id: "",
  transition: transition,
  updated_at: "2022-05-17T17:39:01.382927298Z",
  workspace_name: "test-workspace",
  workspace_owner_id: MockUser.id,
  workspace_owner_name: MockUser.username,
  workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
  deadline: "2022-05-17T23:39:00.00Z",
  reason: "initiator",
  resources: [],
  status: "running",
})

export const MockWorkspaceBuildStop: TypesGen.WorkspaceBuild = {
  ...MockWorkspaceBuild,
  id: "2",
  transition: "stop",
}

export const MockWorkspaceBuildDelete: TypesGen.WorkspaceBuild = {
  ...MockWorkspaceBuild,
  id: "3",
  transition: "delete",
}

export const MockBuilds = [
  MockWorkspaceBuild,
  MockWorkspaceBuildStop,
  MockWorkspaceBuildDelete,
]

export const MockWorkspace: TypesGen.Workspace = {
  id: "test-workspace",
  name: "Test-Workspace",
  created_at: "",
  updated_at: "",
  template_id: MockTemplate.id,
  template_name: MockTemplate.name,
  template_icon: MockTemplate.icon,
  outdated: false,
  owner_id: MockUser.id,
  owner_name: MockUser.username,
  autostart_schedule: MockWorkspaceAutostartEnabled.schedule,
  ttl_ms: 2 * 60 * 60 * 1000, // 2 hours as milliseconds
  latest_build: MockWorkspaceBuild,
  last_used_at: "",
}

export const MockStoppedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-stopped-workspace",
  latest_build: { ...MockWorkspaceBuildStop, status: "stopped" },
}
export const MockStoppingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-stopping-workspace",
  latest_build: {
    ...MockWorkspaceBuildStop,
    job: MockRunningProvisionerJob,
    status: "stopping",
  },
}
export const MockStartingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-starting-workspace",
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockRunningProvisionerJob,
    transition: "start",
    status: "starting",
  },
}
export const MockCancelingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-canceling-workspace",
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockCancelingProvisionerJob,
    status: "canceling",
  },
}
export const MockCanceledWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-canceled-workspace",
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockCanceledProvisionerJob,
    status: "canceled",
  },
}
export const MockFailedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-failed-workspace",
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockFailedProvisionerJob,
    status: "failed",
  },
}
export const MockDeletingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-deleting-workspace",
  latest_build: {
    ...MockWorkspaceBuildDelete,
    job: MockRunningProvisionerJob,
    status: "deleting",
  },
}
export const MockDeletedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-deleted-workspace",
  latest_build: { ...MockWorkspaceBuildDelete, status: "deleted" },
}

export const MockOutdatedWorkspace: TypesGen.Workspace = {
  ...MockFailedWorkspace,
  id: "test-outdated-workspace",
  outdated: true,
}

export const MockPendingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  id: "test-pending-workspace",
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockPendingProvisionerJob,
    transition: "start",
    status: "pending",
  },
}

// requests the MockWorkspace
export const MockWorkspaceRequest: TypesGen.CreateWorkspaceRequest = {
  name: "test",
  parameter_values: [],
  template_id: "test-template",
}

export const MockWorkspaceCountResponse: TypesGen.WorkspaceCountResponse = {
  count: 26, // just over 1 page
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
  oidc: false,
}

export const MockGitSSHKey: TypesGen.GitSSHKey = {
  user_id: "1fa0200f-7331-4524-a364-35770666caa7",
  created_at: "2022-05-16T14:30:34.148205897Z",
  updated_at: "2022-05-16T15:29:10.302441433Z",
  public_key:
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFJOQRIM7kE30rOzrfy+/+R+nQGCk7S9pioihy+2ARbq",
}

export const MockWorkspaceBuildLogs: TypesGen.ProvisionerJobLog[] = [
  {
    id: 1,
    created_at: "2022-05-19T16:45:31.005Z",
    log_source: "provisioner_daemon",
    log_level: "info",
    stage: "Setting up",
    output: "",
  },
  {
    id: 2,
    created_at: "2022-05-19T16:45:31.006Z",
    log_source: "provisioner_daemon",
    log_level: "info",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: 3,
    created_at: "2022-05-19T16:45:31.072Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: 4,
    created_at: "2022-05-19T16:45:31.073Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Initializing the backend...",
  },
  {
    id: 5,
    created_at: "2022-05-19T16:45:31.077Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: 6,
    created_at: "2022-05-19T16:45:31.078Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Initializing provider plugins...",
  },
  {
    id: 7,
    created_at: "2022-05-19T16:45:31.078Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: '- Finding hashicorp/google versions matching "~\u003e 4.15"...',
  },
  {
    id: 8,
    created_at: "2022-05-19T16:45:31.123Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: '- Finding coder/coder versions matching "0.3.4"...',
  },
  {
    id: 9,
    created_at: "2022-05-19T16:45:31.137Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "- Using hashicorp/google v4.21.0 from the shared cache directory",
  },
  {
    id: 10,
    created_at: "2022-05-19T16:45:31.344Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "- Using coder/coder v0.3.4 from the shared cache directory",
  },
  {
    id: 11,
    created_at: "2022-05-19T16:45:31.388Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: 12,
    created_at: "2022-05-19T16:45:31.388Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output:
      "Terraform has created a lock file .terraform.lock.hcl to record the provider",
  },
  {
    id: 13,
    created_at: "2022-05-19T16:45:31.389Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output:
      "selections it made above. Include this file in your version control repository",
  },
  {
    id: 14,
    created_at: "2022-05-19T16:45:31.389Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output:
      "so that Terraform can guarantee to make the same selections by default when",
  },
  {
    id: 15,
    created_at: "2022-05-19T16:45:31.39Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: 'you run "terraform init" in the future.',
  },
  {
    id: 16,
    created_at: "2022-05-19T16:45:31.39Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: 17,
    created_at: "2022-05-19T16:45:31.391Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Terraform has been successfully initialized!",
  },
  {
    id: 18,
    created_at: "2022-05-19T16:45:31.42Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Terraform 1.1.9",
  },
  {
    id: 19,
    created_at: "2022-05-19T16:45:33.537Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "coder_agent.dev: Plan to create",
  },
  {
    id: 20,
    created_at: "2022-05-19T16:45:33.537Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Plan to create",
  },
  {
    id: 21,
    created_at: "2022-05-19T16:45:33.538Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Plan to create",
  },
  {
    id: 22,
    created_at: "2022-05-19T16:45:33.539Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Plan: 3 to add, 0 to change, 0 to destroy.",
  },
  {
    id: 23,
    created_at: "2022-05-19T16:45:33.712Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "coder_agent.dev: Creating...",
  },
  {
    id: 24,
    created_at: "2022-05-19T16:45:33.719Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output:
      "coder_agent.dev: Creation complete after 0s [id=d07f5bdc-4a8d-4919-9cdb-0ac6ba9e64d6]",
  },
  {
    id: 25,
    created_at: "2022-05-19T16:45:34.139Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Creating...",
  },
  {
    id: 26,
    created_at: "2022-05-19T16:45:44.14Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Still creating... [10s elapsed]",
  },
  {
    id: 27,
    created_at: "2022-05-19T16:45:47.106Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output:
      "google_compute_disk.root: Creation complete after 13s [id=projects/bruno-coder-v2/zones/europe-west4-b/disks/coder-developer-bruno-dev-123-root]",
  },
  {
    id: 28,
    created_at: "2022-05-19T16:45:47.118Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Creating...",
  },
  {
    id: 29,
    created_at: "2022-05-19T16:45:57.122Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Still creating... [10s elapsed]",
  },
  {
    id: 30,
    created_at: "2022-05-19T16:46:00.837Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output:
      "google_compute_instance.dev[0]: Creation complete after 14s [id=projects/bruno-coder-v2/zones/europe-west4-b/instances/coder-developer-bruno-dev-123]",
  },
  {
    id: 31,
    created_at: "2022-05-19T16:46:00.846Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Apply complete! Resources: 3 added, 0 changed, 0 destroyed.",
  },
  {
    id: 32,
    created_at: "2022-05-19T16:46:00.847Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Outputs: 0",
  },
  {
    id: 33,
    created_at: "2022-05-19T16:46:02.283Z",
    log_source: "provisioner_daemon",
    log_level: "info",
    stage: "Cleaning Up",
    output: "",
  },
]

export const MockCancellationMessage = {
  message: "Job successfully canceled",
}

type MockAPIInput = {
  message?: string
  detail?: string
  validations?: FieldError[]
}

type MockAPIOutput = {
  response: {
    data: {
      message: string
      detail: string | undefined
      validations: FieldError[] | undefined
    }
  }
  isAxiosError: boolean
}

type MakeMockApiErrorFunction = (input: MockAPIInput) => MockAPIOutput

export const makeMockApiError: MakeMockApiErrorFunction = ({
  message,
  detail,
  validations,
}) => ({
  response: {
    data: {
      message: message ?? "Something went wrong.",
      detail: detail ?? undefined,
      validations: validations ?? undefined,
    },
  },
  isAxiosError: true,
})

export const MockEntitlements: TypesGen.Entitlements = {
  errors: [],
  warnings: [],
  has_license: false,
  features: {},
  experimental: false,
  trial: false,
}

export const MockEntitlementsWithWarnings: TypesGen.Entitlements = {
  errors: [],
  warnings: ["You are over your active user limit.", "And another thing."],
  has_license: true,
  experimental: false,
  trial: false,
  features: {
    user_limit: {
      enabled: true,
      entitlement: "grace_period",
      limit: 100,
      actual: 102,
    },
    audit_log: {
      enabled: true,
      entitlement: "entitled",
    },
    browser_only: {
      enabled: true,
      entitlement: "entitled",
    },
  },
}

export const MockEntitlementsWithAuditLog: TypesGen.Entitlements = {
  errors: [],
  warnings: [],
  has_license: true,
  experimental: false,
  trial: false,
  features: {
    audit_log: {
      enabled: true,
      entitlement: "entitled",
    },
  },
}

export const MockAuditLog: TypesGen.AuditLog = {
  id: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
  request_id: "53bded77-7b9d-4e82-8771-991a34d759f9",
  time: "2022-05-19T16:45:57.122Z",
  organization_id: "fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0",
  ip: "127.0.0.1",
  user_agent:
    '"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36"',
  resource_type: "workspace",
  resource_id: "ef8d1cf4-82de-4fd9-8980-047dad6d06b5",
  resource_target: "bruno-dev",
  resource_icon: "",
  action: "create",
  diff: {
    ttl: {
      old: 0,
      new: 3600000000000,
      secret: false,
    },
  },
  status_code: 200,
  additional_fields: {},
  description: "{user} updated workspace {target}",
  user: MockUser,
}

export const MockAuditLog2: TypesGen.AuditLog = {
  ...MockAuditLog,
  id: "53bded77-7b9d-4e82-8771-991a34d759f9",
  action: "write",
  diff: {
    workspace_name: {
      old: "old-workspace-name",
      new: MockWorkspace.name,
      secret: false,
    },
    workspace_auto_off: {
      old: true,
      new: false,
      secret: false,
    },
    template_version_id: {
      old: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
      new: "53bded77-7b9d-4e82-8771-991a34d759f9",
      secret: false,
    },
    roles: {
      old: null,
      new: ["admin", "auditor"],
      secret: false,
    },
  },
}

export const MockAuditLogWithWorkspaceBuild: TypesGen.AuditLog = {
  ...MockAuditLog,
  id: "f90995bf-4a2b-4089-b597-e66e025e523e",
  request_id: "61555889-2875-475c-8494-f7693dd5d75b",
  action: "stop",
  resource_type: "workspace_build",
  description: "{user} stopped workspace build for {target}",
  additional_fields: {
    workspaceName: "test2",
  },
}

export const MockWorkspaceQuota: TypesGen.WorkspaceQuota = {
  user_workspace_count: 0,
  user_workspace_limit: 100,
}

export const MockGroup: TypesGen.Group = {
  id: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
  name: "Front-End",
  avatar_url: "https://example.com",
  organization_id: MockOrganization.id,
  members: [MockUser, MockUser2],
}

export const MockTemplateACL: TypesGen.TemplateACL = {
  group: [
    { ...everyOneGroup(MockOrganization.id), role: "use" },
    { ...MockGroup, role: "admin" },
  ],
  users: [{ ...MockUser, role: "use" }],
}

export const MockTemplateACLEmpty: TypesGen.TemplateACL = {
  group: [],
  users: [],
}
