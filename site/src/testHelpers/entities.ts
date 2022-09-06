import { FieldError } from "api/errors"
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

export const MockSiteRoles = [MockUserAdminRole, MockAuditorRole]

// assignableRole takes a role and a boolean. The boolean implies if the
// actor can assign (add/remove) the role from other users.
export function assignableRole(role: TypesGen.Role, assignable: boolean): TypesGen.AssignableRoles {
  return {
    ...role,
    assignable: assignable,
  }
}

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
  avatar_url: "https://github.com/coder.png",
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
  storage_source: "asdf",
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
  created_by_id: "test-creator-id",
  created_by_name: "test_creator",
}

export const MockTemplate: TypesGen.Template = {
  id: "test-template",
  created_at: "2022-05-17T17:39:01.382927298Z",
  updated_at: "2022-05-18T17:39:01.382927298Z",
  organization_id: MockOrganization.id,
  name: "test-template",
  provisioner: MockProvisioner.provisioners[0],
  active_version_id: MockTemplateVersion.id,
  workspace_owner_count: 1,
  description: "This is a test description.",
  max_ttl_ms: 24 * 60 * 60 * 1000,
  min_autostart_interval_ms: 60 * 60 * 1000,
  created_by_id: "test-creator-id",
  created_by_name: "test_creator",
  icon: "/icon/code.svg",
}

export const MockWorkspaceAutostartDisabled: TypesGen.UpdateWorkspaceAutostartRequest = {
  schedule: "",
}

export const MockWorkspaceAutostartEnabled: TypesGen.UpdateWorkspaceAutostartRequest = {
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
  name: "a-workspace-build",
  template_version_id: "",
  transition: "start",
  updated_at: "2022-05-17T17:39:01.382927298Z",
  workspace_name: "test-workspace",
  workspace_owner_id: MockUser.id,
  workspace_owner_name: MockUser.username,
  workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
  deadline: "2022-05-17T23:39:00.00Z",
  reason: "initiator",
}

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

export const MockBuilds = [MockWorkspaceBuild, MockWorkspaceBuildStop, MockWorkspaceBuildDelete]

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
  latest_build: MockWorkspaceBuildStop,
}
export const MockStoppingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: {
    ...MockWorkspaceBuildStop,
    job: MockRunningProvisionerJob,
  },
}
export const MockStartingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockRunningProvisionerJob,
    transition: "start",
  },
}
export const MockCancelingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuild, job: MockCancelingProvisionerJob },
}
export const MockCanceledWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuild, job: MockCanceledProvisionerJob },
}
export const MockFailedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockFailedProvisionerJob,
  },
}
export const MockDeletingWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: { ...MockWorkspaceBuildDelete, job: MockRunningProvisionerJob },
}
export const MockDeletedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: MockWorkspaceBuildDelete,
}

export const MockOutdatedWorkspace: TypesGen.Workspace = { ...MockFailedWorkspace, outdated: true }

export const MockQueuedWorkspace: TypesGen.Workspace = {
  ...MockWorkspace,
  latest_build: {
    ...MockWorkspaceBuild,
    job: MockPendingProvisionerJob,
    transition: "start",
  },
}

export const MockWorkspaceApp: TypesGen.WorkspaceApp = {
  id: "test-app",
  name: "test-app",
  icon: "",
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
  id: "test-workspace-agent-2",
  name: "another-workspace-agent",
  status: "connecting",
  version: "",
  latency: {},
}

export const MockWorkspaceResource: TypesGen.WorkspaceResource = {
  agents: [MockWorkspaceAgent, MockWorkspaceAgentConnecting, MockWorkspaceAgentOutdated],
  created_at: "",
  id: "test-workspace-resource",
  job_id: "",
  name: "a-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
  metadata: [
    { key: "type", value: "a-workspace-resource", sensitive: false },
    { key: "api_key", value: "12345678", sensitive: true },
  ],
}

export const MockWorkspaceResource2: TypesGen.WorkspaceResource = {
  agents: [MockWorkspaceAgent, MockWorkspaceAgentDisconnected, MockWorkspaceAgentOutdated],
  created_at: "",
  id: "test-workspace-resource-2",
  job_id: "",
  name: "another-workspace-resource",
  type: "google_compute_disk",
  workspace_transition: "start",
  metadata: [
    { key: "type", value: "google_compute_disk", sensitive: false },
    { key: "size", value: "32GB", sensitive: false },
  ],
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
  public_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFJOQRIM7kE30rOzrfy+/+R+nQGCk7S9pioihy+2ARbq",
}

export const MockWorkspaceBuildLogs: TypesGen.ProvisionerJobLog[] = [
  {
    id: "836f8ab6-5202-4711-afa5-293394ced011",
    created_at: "2022-05-19T16:45:31.005Z",
    log_source: "provisioner_daemon",
    log_level: "info",
    stage: "Setting up",
    output: "",
  },
  {
    id: "2db0ae92-b310-4a6e-8b1f-23380b70ac7f",
    created_at: "2022-05-19T16:45:31.006Z",
    log_source: "provisioner_daemon",
    log_level: "info",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: "37a5b7b1-b3eb-47cf-b80b-bd16e2e08a3d",
    created_at: "2022-05-19T16:45:31.072Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: "5e4e37a1-c217-48bc-84f5-7f1c3efbd042",
    created_at: "2022-05-19T16:45:31.073Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Initializing the backend...",
  },
  {
    id: "060ed132-5d12-4584-9005-5c9557febe2f",
    created_at: "2022-05-19T16:45:31.077Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: "b2e70a1c-1943-4616-8ac9-25326c9f7e7b",
    created_at: "2022-05-19T16:45:31.078Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Initializing provider plugins...",
  },
  {
    id: "993107fe-6dfb-42ec-912a-b32f50e60d62",
    created_at: "2022-05-19T16:45:31.078Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: '- Finding hashicorp/google versions matching "~\u003e 4.15"...',
  },
  {
    id: "2ad2e2a1-7a75-4827-8cb9-928acfc6fc07",
    created_at: "2022-05-19T16:45:31.123Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: '- Finding coder/coder versions matching "0.3.4"...',
  },
  {
    id: "7c723a90-0190-4c2f-9d97-ede39ef3d55f",
    created_at: "2022-05-19T16:45:31.137Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "- Using hashicorp/google v4.21.0 from the shared cache directory",
  },
  {
    id: "3910144b-411b-4a53-9900-88d406ed9bf4",
    created_at: "2022-05-19T16:45:31.344Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "- Using coder/coder v0.3.4 from the shared cache directory",
  },
  {
    id: "e3a02ad4-edc0-442f-8b9a-39d01d56b43b",
    created_at: "2022-05-19T16:45:31.388Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: "440cceb3-aabf-4838-979b-1fd37fe2d8d8",
    created_at: "2022-05-19T16:45:31.388Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Terraform has created a lock file .terraform.lock.hcl to record the provider",
  },
  {
    id: "90e1f244-78ff-4d95-871e-b2bebcabc39a",
    created_at: "2022-05-19T16:45:31.389Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "selections it made above. Include this file in your version control repository",
  },
  {
    id: "e4527d6c-2412-452b-a946-5870787caf6b",
    created_at: "2022-05-19T16:45:31.389Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "so that Terraform can guarantee to make the same selections by default when",
  },
  {
    id: "02f96d19-d94b-4d0e-a1c4-313a0d2ff9e3",
    created_at: "2022-05-19T16:45:31.39Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: 'you run "terraform init" in the future.',
  },
  {
    id: "667c03ca-1b24-4f36-a598-f0322cf3e2a1",
    created_at: "2022-05-19T16:45:31.39Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "",
  },
  {
    id: "48039d6a-9b21-460f-9ca3-4b0e2becfd18",
    created_at: "2022-05-19T16:45:31.391Z",
    log_source: "provisioner",
    log_level: "debug",
    stage: "Starting workspace",
    output: "Terraform has been successfully initialized!",
  },
  {
    id: "6fe4b64f-3aa6-4850-96e9-6db8478a53be",
    created_at: "2022-05-19T16:45:31.42Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Terraform 1.1.9",
  },
  {
    id: "fa7b6321-7ecd-492d-a671-6366186fad08",
    created_at: "2022-05-19T16:45:33.537Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "coder_agent.dev: Plan to create",
  },
  {
    id: "e677e49f-c5ba-417c-8c9d-78bdad744ce1",
    created_at: "2022-05-19T16:45:33.537Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Plan to create",
  },
  {
    id: "4b0e6168-29e4-4419-bf81-b57e31087666",
    created_at: "2022-05-19T16:45:33.538Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Plan to create",
  },
  {
    id: "5902f89c-8acd-45e2-9bd6-de4d6fd8fc9c",
    created_at: "2022-05-19T16:45:33.539Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Plan: 3 to add, 0 to change, 0 to destroy.",
  },
  {
    id: "a8107907-7c53-4aae-bb48-9a5f9759c7d5",
    created_at: "2022-05-19T16:45:33.712Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "coder_agent.dev: Creating...",
  },
  {
    id: "aaf13503-2f1a-4f6c-aced-b8fc48304dc1",
    created_at: "2022-05-19T16:45:33.719Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "coder_agent.dev: Creation complete after 0s [id=d07f5bdc-4a8d-4919-9cdb-0ac6ba9e64d6]",
  },
  {
    id: "4ada8886-f5b3-4fee-a1a3-72064b50d5ae",
    created_at: "2022-05-19T16:45:34.139Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Creating...",
  },
  {
    id: "8ffc59e8-a4d0-4ffe-9bcc-cb84ca51cc22",
    created_at: "2022-05-19T16:45:44.14Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_disk.root: Still creating... [10s elapsed]",
  },
  {
    id: "063189fd-75ad-415a-ac77-8c34b9e202b2",
    created_at: "2022-05-19T16:45:47.106Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output:
      "google_compute_disk.root: Creation complete after 13s [id=projects/bruno-coder-v2/zones/europe-west4-b/disks/coder-developer-bruno-dev-123-root]",
  },
  {
    id: "6fd554a1-a7a2-439f-b8d8-369d6c1ead21",
    created_at: "2022-05-19T16:45:47.118Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Creating...",
  },
  {
    id: "87388f7e-ab01-44b1-b35e-8e06636164d3",
    created_at: "2022-05-19T16:45:57.122Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "google_compute_instance.dev[0]: Still creating... [10s elapsed]",
  },
  {
    id: "baa40120-3f18-40d2-a35c-b11f421a1ce1",
    created_at: "2022-05-19T16:46:00.837Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output:
      "google_compute_instance.dev[0]: Creation complete after 14s [id=projects/bruno-coder-v2/zones/europe-west4-b/instances/coder-developer-bruno-dev-123]",
  },
  {
    id: "00e18953-fba6-4b43-97a3-ecf376553c08",
    created_at: "2022-05-19T16:46:00.846Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Apply complete! Resources: 3 added, 0 changed, 0 destroyed.",
  },
  {
    id: "431811da-b534-4d92-b6e5-44814548c812",
    created_at: "2022-05-19T16:46:00.847Z",
    log_source: "provisioner",
    log_level: "info",
    stage: "Starting workspace",
    output: "Outputs: 0",
  },
  {
    id: "70459334-4878-4bda-a546-98eee166c4c6",
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

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const makeMockApiError = ({
  message,
  detail,
  validations,
}: {
  message?: string
  detail?: string
  validations?: FieldError[]
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
  warnings: [],
  has_license: false,
  features: {},
}

export const MockEntitlementsWithWarnings: TypesGen.Entitlements = {
  warnings: ["You are over your active user limit.", "And another thing."],
  has_license: true,
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
  },
}

export const MockEntitlementsWithAuditLog: TypesGen.Entitlements = {
  warnings: [],
  has_license: true,
  features: {
    audit_log: {
      enabled: true,
      entitlement: "entitled",
    },
  },
}
