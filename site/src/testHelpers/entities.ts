import { withDefaultFeatures } from "./../api/api"
import { FieldError } from "api/errors"
import { everyOneGroup } from "util/groups"
import * as Types from "../api/types"
import * as TypesGen from "../api/typesGenerated"
import range from "lodash/range"
import { Permissions } from "xServices/auth/authXService"
import { TemplateVersionFiles } from "util/templateVersion"
import { FileTree } from "util/filetree"

export const MockTemplateDAUResponse: TypesGen.TemplateDAUsResponse = {
  entries: [
    { date: "2022-08-27T00:00:00Z", amount: 1 },
    { date: "2022-08-29T00:00:00Z", amount: 2 },
    { date: "2022-08-30T00:00:00Z", amount: 1 },
  ],
}
export const MockDeploymentDAUResponse: TypesGen.DeploymentDAUsResponse = {
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

export const MockTokens: TypesGen.APIKey[] = [
  {
    id: "tBoVE3dqLl",
    user_id: "f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
    last_used: "0001-01-01T00:00:00Z",
    expires_at: "2023-01-15T20:10:45.637438Z",
    created_at: "2022-12-16T20:10:45.637452Z",
    updated_at: "2022-12-16T20:10:45.637452Z",
    login_type: "token",
    scope: "all",
    lifetime_seconds: 2592000,
  },
  {
    id: "tBoVE3dqLl",
    user_id: "f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
    last_used: "0001-01-01T00:00:00Z",
    expires_at: "2023-01-15T20:10:45.637438Z",
    created_at: "2022-12-16T20:10:45.637452Z",
    updated_at: "2022-12-16T20:10:45.637452Z",
    login_type: "token",
    scope: "all",
    lifetime_seconds: 2592000,
  },
]

export const MockBuildInfo: TypesGen.BuildInfoResponse = {
  external_url: "file:///mock-url",
  version: "v99.999.9999+c9cdf14",
}

export const MockUpdateCheck: TypesGen.UpdateCheckResponse = {
  current: true,
  url: "file:///mock-url",
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
  tags: {},
}

export const MockProvisionerJob: TypesGen.ProvisionerJob = {
  created_at: "",
  id: "test-provisioner-job",
  status: "succeeded",
  file_id: "fc0774ce-cc9e-48d4-80ae-88f7a4d4a8b0",
  completed_at: "2022-05-17T17:39:01.382927298Z",
  tags: {},
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

export const MockTemplateVersion2: TypesGen.TemplateVersion = {
  id: "test-template-version-2",
  created_at: "2022-05-17T17:39:01.382927298Z",
  updated_at: "2022-05-17T17:39:01.382927298Z",
  template_id: "test-template",
  job: MockProvisionerJob,
  name: "test-version-2",
  readme: `---
name:Template test 2
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
  display_name: "Test Template",
  provisioner: MockProvisioner.provisioners[0],
  active_version_id: MockTemplateVersion.id,
  active_user_count: 1,
  build_time_stats: {
    start: {
      P50: 1000,
      P95: 1500,
    },
    stop: {
      P50: 1000,
      P95: 1500,
    },
    delete: {
      P50: 1000,
      P95: 1500,
    },
  },
  description: "This is a test description.",
  default_ttl_ms: 24 * 60 * 60 * 1000,
  created_by_id: "test-creator-id",
  created_by_name: "test_creator",
  icon: "/icon/code.svg",
  allow_user_cancel_workspace_jobs: true,
}

export const MockTemplateVersionFiles: TemplateVersionFiles = {
  "README.md": "# Example\n\nThis is an example template.",
  "main.tf": `// Provides info about the workspace.
data "coder_workspace" "me" {}

// Provides the startup script used to download
// the agent and communicate with Coder.
resource "coder_agent" "dev" {
os = "linux"
arch = "amd64"
}

resource "kubernetes_pod" "main" {
// Ensures that the Pod dies when the workspace shuts down!
count = data.coder_workspace.me.start_count
metadata {
  name      = "dev-\${data.coder_workspace.me.id}"
}
spec {
  container {
    image   = "ubuntu"
    command = ["sh", "-c", coder_agent.main.init_script]
    env {
      name  = "CODER_AGENT_TOKEN"
      value = coder_agent.main.token
    }
  }
}
}
`,
}

export const MockTemplateVersionFileTree: FileTree = {
  "README.md": "# Example\n\nThis is an example template.",
  "main.tf": `// Provides info about the workspace.
data "coder_workspace" "me" {}

// Provides the startup script used to download
// the agent and communicate with Coder.
resource "coder_agent" "dev" {
os = "linux"
arch = "amd64"
}

resource "kubernetes_pod" "main" {
// Ensures that the Pod dies when the workspace shuts down!
count = data.coder_workspace.me.start_count
metadata {
  name      = "dev-\${data.coder_workspace.me.id}"
}
spec {
  container {
    image   = "ubuntu"
    command = ["sh", "-c", coder_agent.main.init_script]
    env {
      name  = "CODER_AGENT_TOKEN"
      value = coder_agent.main.token
    }
  }
}
}
`,
  images: {
    "java.Dockerfile": "FROM eclipse-temurin:17-jdk-jammy",
    "python.Dockerfile": "FROM python:3.8-slim-buster",
  },
}

export const MockWorkspaceApp: TypesGen.WorkspaceApp = {
  id: "test-app",
  slug: "test-app",
  display_name: "Test App",
  icon: "",
  subdomain: false,
  health: "disabled",
  external: false,
  url: "",
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
  connection_timeout_seconds: 120,
  troubleshooting_url: "https://coder.com/troubleshoot",
  lifecycle_state: "starting",
  login_before_ready: false,
  startup_script_timeout_seconds: 120,
}

export const MockWorkspaceAgentDisconnected: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-2",
  name: "another-workspace-agent",
  status: "disconnected",
  version: "",
  latency: {},
  lifecycle_state: "ready",
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
  lifecycle_state: "ready",
}

export const MockWorkspaceAgentConnecting: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-connecting",
  name: "another-workspace-agent",
  status: "connecting",
  version: "",
  latency: {},
  lifecycle_state: "created",
}

export const MockWorkspaceAgentTimeout: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-timeout",
  name: "a-timed-out-workspace-agent",
  status: "timeout",
  version: "",
  latency: {},
  lifecycle_state: "created",
}

export const MockWorkspaceAgentStarting: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-starting",
  name: "a-starting-workspace-agent",
  lifecycle_state: "starting",
}

export const MockWorkspaceAgentStartTimeout: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-start-timeout",
  name: "a-workspace-agent-timed-out-while-running-startup-script",
  lifecycle_state: "start_timeout",
}

export const MockWorkspaceAgentStartError: TypesGen.WorkspaceAgent = {
  ...MockWorkspaceAgent,
  id: "test-workspace-agent-start-error",
  name: "a-workspace-agent-errored-while-running-startup-script",
  lifecycle_state: "start_error",
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
  metadata: [{ key: "api_key", value: "12345678", sensitive: true }],
  daily_cost: 10,
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
  metadata: [{ key: "size", value: "32GB", sensitive: false }],
  daily_cost: 10,
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
  metadata: [{ key: "size", value: "32GB", sensitive: false }],
  daily_cost: 20,
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
  template_version_id: MockTemplateVersion.id,
  template_version_name: MockTemplateVersion.name,
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
  daily_cost: 20,
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
  template_version_id: MockTemplateVersion.id,
  template_version_name: MockTemplateVersion.name,
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
  daily_cost: 20,
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
  template_display_name: MockTemplate.display_name,
  template_allow_user_cancel_workspace_jobs:
    MockTemplate.allow_user_cancel_workspace_jobs,
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

// just over one page of workspaces
export const MockWorkspacesResponse: TypesGen.WorkspacesResponse = {
  workspaces: range(1, 27).map((id: number) => ({
    ...MockWorkspace,
    id: id.toString(),
    name: `${MockWorkspace.name}${id}`,
  })),
  count: 26,
}

export const MockTemplateVersionParameter1: TypesGen.TemplateVersionParameter =
  {
    name: "first_parameter",
    type: "string",
    description: "This is first parameter",
    description_plaintext: "Markdown: This is first parameter",
    default_value: "abc",
    mutable: true,
    icon: "/icon/folder.svg",
    options: [],
  }

export const MockTemplateVersionParameter2: TypesGen.TemplateVersionParameter =
  {
    name: "second_parameter",
    type: "number",
    description: "This is second parameter",
    description_plaintext: "Markdown: This is second parameter",
    default_value: "2",
    mutable: true,
    icon: "/icon/folder.svg",
    options: [],
    validation_min: 1,
    validation_max: 3,
    validation_monotonic: "increasing",
  }

export const MockTemplateVersionParameter3: TypesGen.TemplateVersionParameter =
  {
    name: "third_parameter",
    type: "string",
    description: "This is third parameter",
    description_plaintext: "Markdown: This is third parameter",
    default_value: "aaa",
    mutable: true,
    icon: "/icon/database.svg",
    options: [],
    validation_error: "No way!",
    validation_regex: "^[a-z]{3}$",
  }

export const MockTemplateVersionParameter4: TypesGen.TemplateVersionParameter =
  {
    name: "fourth_parameter",
    type: "string",
    description: "This is fourth parameter",
    description_plaintext: "Markdown: This is fourth parameter",
    default_value: "def",
    mutable: false,
    icon: "/icon/database.svg",
    options: [],
  }

export const MockTemplateVersionParameter5: TypesGen.TemplateVersionParameter =
  {
    name: "fifth_parameter",
    type: "number",
    description: "This is fifth parameter",
    description_plaintext: "Markdown: This is fifth parameter",
    default_value: "5",
    mutable: true,
    icon: "/icon/folder.svg",
    options: [],
    validation_min: 1,
    validation_max: 10,
    validation_monotonic: "decreasing",
  }

// requests the MockWorkspace
export const MockWorkspaceRequest: TypesGen.CreateWorkspaceRequest = {
  name: "test",
  parameter_values: [],
  template_id: "test-template",
  rich_parameter_values: [
    {
      name: MockTemplateVersionParameter1.name,
      value: MockTemplateVersionParameter1.default_value,
    },
  ],
}

export const MockUserAgent: Types.UserAgent = {
  browser: "Chrome 99.0.4844",
  device: "Other",
  ip_address: "11.22.33.44",
  os: "Windows 10",
}

export const MockAuthMethods: TypesGen.AuthMethods = {
  password: { enabled: true },
  github: { enabled: false },
  oidc: { enabled: false, signInText: "", iconUrl: "" },
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
  features: withDefaultFeatures({}),
  experimental: false,
  require_telemetry: false,
  trial: false,
}

export const MockEntitlementsWithWarnings: TypesGen.Entitlements = {
  errors: [],
  warnings: ["You are over your active user limit.", "And another thing."],
  has_license: true,
  experimental: false,
  trial: false,
  require_telemetry: false,
  features: withDefaultFeatures({
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
  }),
}

export const MockEntitlementsWithAuditLog: TypesGen.Entitlements = {
  errors: [],
  warnings: [],
  has_license: true,
  experimental: false,
  require_telemetry: false,
  trial: false,
  features: withDefaultFeatures({
    audit_log: {
      enabled: true,
      entitlement: "entitled",
    },
  }),
}

export const MockExperiments: TypesGen.Experiment[] = []

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
  description: "{user} created workspace {target}",
  user: MockUser,
  resource_link: "/@admin/bruno-dev",
  is_deleted: false,
}

export const MockAuditLog2: TypesGen.AuditLog = {
  ...MockAuditLog,
  id: "53bded77-7b9d-4e82-8771-991a34d759f9",
  action: "write",
  time: "2022-05-20T16:45:57.122Z",
  description: "{user} updated workspace {target}",
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

export const MockWorkspaceCreateAuditLogForDifferentOwner = {
  ...MockAuditLog,
  additional_fields: {
    workspace_owner: "Member",
  },
}

export const MockAuditLogWithWorkspaceBuild: TypesGen.AuditLog = {
  ...MockAuditLog,
  id: "f90995bf-4a2b-4089-b597-e66e025e523e",
  request_id: "61555889-2875-475c-8494-f7693dd5d75b",
  action: "stop",
  resource_type: "workspace_build",
  description: "{user} stopped build for workspace {target}",
  additional_fields: {
    workspace_name: "test2",
  },
}

export const MockAuditLogWithDeletedResource: TypesGen.AuditLog = {
  ...MockAuditLog,
  is_deleted: true,
}

export const MockAuditLogGitSSH: TypesGen.AuditLog = {
  ...MockAuditLog,
  diff: {
    private_key: {
      old: "",
      new: "",
      secret: true,
    },
    public_key: {
      old: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINRUPjBSNtOAnL22+r07OSu9t3Lnm8/5OX8bRHECKS9g\n",
      new: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEwoUPJPMekuSzMZyV0rA82TGGNzw/Uj/dhLbwiczTpV\n",
      secret: false,
    },
  },
}

export const MockAuditLogSuccessfulLogin: TypesGen.AuditLog = {
  ...MockAuditLog,
  resource_type: "api_key",
  resource_target: "",
  action: "login",
  status_code: 201,
  description: "{user} logged in",
}

export const MockAuditLogUnsuccessfulLoginKnownUser: TypesGen.AuditLog = {
  ...MockAuditLogSuccessfulLogin,
  status_code: 401,
}

export const MockAuditLogUnsuccessfulLoginUnknownUser: TypesGen.AuditLog = {
  ...MockAuditLogSuccessfulLogin,
  status_code: 401,
  user: undefined,
}

export const MockWorkspaceQuota: TypesGen.WorkspaceQuota = {
  credits_consumed: 0,
  budget: 100,
}

export const MockGroup: TypesGen.Group = {
  id: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
  name: "Front-End",
  avatar_url: "https://example.com",
  organization_id: MockOrganization.id,
  members: [MockUser, MockUser2],
  quota_allowance: 5,
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

export const MockTemplateExample: TypesGen.TemplateExample = {
  id: "aws-windows",
  url: "https://github.com/coder/coder/tree/main/examples/templates/aws-windows",
  name: "Develop in an ECS-hosted container",
  description: "Get started with Linux development on AWS ECS.",
  markdown:
    "\n# aws-ecs\n\nThis is a sample template for running a Coder workspace on ECS. It assumes there\nis a pre-existing ECS cluster with EC2-based compute to host the workspace.\n\n## Architecture\n\nThis workspace is built using the following AWS resources:\n\n- Task definition - the container definition, includes the image, command, volume(s)\n- ECS service - manages the task definition\n\n## code-server\n\n`code-server` is installed via the `startup_script` argument in the `coder_agent`\nresource block. The `coder_app` resource is defined to access `code-server` through\nthe dashboard UI over `localhost:13337`.\n",
  icon: "/icon/aws.png",
  tags: ["aws", "cloud"],
}

export const MockTemplateExample2: TypesGen.TemplateExample = {
  id: "aws-linux",
  url: "https://github.com/coder/coder/tree/main/examples/templates/aws-linux",
  name: "Develop in Linux on AWS EC2",
  description: "Get started with Linux development on AWS EC2.",
  markdown:
    '\n# aws-linux\n\nTo get started, run `coder templates init`. When prompted, select this template.\nFollow the on-screen instructions to proceed.\n\n## Authentication\n\nThis template assumes that coderd is run in an environment that is authenticated\nwith AWS. For example, run `aws configure import` to import credentials on the\nsystem and user running coderd.  For other ways to authenticate [consult the\nTerraform docs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#authentication-and-configuration).\n\n## Required permissions / policy\n\nThe following sample policy allows Coder to create EC2 instances and modify\ninstances provisioned by Coder:\n\n```json\n{\n    "Version": "2012-10-17",\n    "Statement": [\n        {\n            "Sid": "VisualEditor0",\n            "Effect": "Allow",\n            "Action": [\n                "ec2:GetDefaultCreditSpecification",\n                "ec2:DescribeIamInstanceProfileAssociations",\n                "ec2:DescribeTags",\n                "ec2:CreateTags",\n                "ec2:RunInstances",\n                "ec2:DescribeInstanceCreditSpecifications",\n                "ec2:DescribeImages",\n                "ec2:ModifyDefaultCreditSpecification",\n                "ec2:DescribeVolumes"\n            ],\n            "Resource": "*"\n        },\n        {\n            "Sid": "CoderResources",\n            "Effect": "Allow",\n            "Action": [\n                "ec2:DescribeInstances",\n                "ec2:DescribeInstanceAttribute",\n                "ec2:UnmonitorInstances",\n                "ec2:TerminateInstances",\n                "ec2:StartInstances",\n                "ec2:StopInstances",\n                "ec2:DeleteTags",\n                "ec2:MonitorInstances",\n                "ec2:CreateTags",\n                "ec2:RunInstances",\n                "ec2:ModifyInstanceAttribute",\n                "ec2:ModifyInstanceCreditSpecification"\n            ],\n            "Resource": "arn:aws:ec2:*:*:instance/*",\n            "Condition": {\n                "StringEquals": {\n                    "aws:ResourceTag/Coder_Provisioned": "true"\n                }\n            }\n        }\n    ]\n}\n```\n\n## code-server\n\n`code-server` is installed via the `startup_script` argument in the `coder_agent`\nresource block. The `coder_app` resource is defined to access `code-server` through\nthe dashboard UI over `localhost:13337`.\n',
  icon: "/icon/aws.png",
  tags: ["aws", "cloud"],
}

export const MockPermissions: Permissions = {
  createGroup: true,
  createTemplates: true,
  createUser: true,
  deleteTemplates: true,
  readAllUsers: true,
  updateUsers: true,
  viewAuditLog: true,
  viewDeploymentConfig: true,
  viewUpdateCheck: true,
}

export const MockAppearance: TypesGen.AppearanceConfig = {
  logo_url: "",
  service_banner: {
    enabled: false,
  },
}

export const MockWorkspaceBuildParameter1: TypesGen.WorkspaceBuildParameter = {
  name: MockTemplateVersionParameter1.name,
  value: "mock-abc",
}

export const MockWorkspaceBuildParameter2: TypesGen.WorkspaceBuildParameter = {
  name: MockTemplateVersionParameter2.name,
  value: "3",
}

export const MockWorkspaceBuildParameter5: TypesGen.WorkspaceBuildParameter = {
  name: MockTemplateVersionParameter5.name,
  value: "5",
}

export const mockParameterSchema = (
  partial: Partial<TypesGen.ParameterSchema>,
): TypesGen.ParameterSchema => {
  return {
    id: "000000",
    job_id: "000000",
    allow_override_destination: false,
    allow_override_source: true,
    created_at: "",
    default_destination_scheme: "none",
    default_refresh: "",
    default_source_scheme: "data",
    default_source_value: "default-value",
    name: "parameter name",
    description: "Some description!",
    redisplay_value: false,
    validation_condition: "",
    validation_contains: [],
    validation_error: "",
    validation_type_system: "",
    validation_value_type: "",
    ...partial,
  }
}
