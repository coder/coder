import {
	type DeploymentConfig,
	type GetLicensesResponse,
	withDefaultFeatures,
} from "api/api";
import type { FieldError } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import type { Permissions } from "contexts/auth/permissions";
import type { ProxyLatencyReport } from "contexts/useProxyLatency";
import range from "lodash/range";
import type { OrganizationPermissions } from "modules/management/organizationPermissions";
import type { FileTree } from "utils/filetree";
import type { TemplateVersionFiles } from "utils/templateVersion";

export const MockOrganization: TypesGen.Organization = {
	id: "my-organization-id",
	name: "my-organization",
	display_name: "My Organization",
	description: "An organization that gets used for stuff.",
	icon: "/emojis/1f957.png",
	created_at: "",
	updated_at: "",
	is_default: false,
};

export const MockDefaultOrganization: TypesGen.Organization = {
	...MockOrganization,
	is_default: true,
};

export const MockOrganization2: TypesGen.Organization = {
	id: "my-organization-2-id",
	name: "my-organization-2",
	display_name: "My Organization 2",
	description: "Another organization that gets used for stuff.",
	icon: "/emojis/1f957.png",
	created_at: "",
	updated_at: "",
	is_default: false,
};

export const MockTemplateDAUResponse: TypesGen.DAUsResponse = {
	tz_hour_offset: 0,
	entries: [
		{ date: "2022-08-27", amount: 1 },
		{ date: "2022-08-29", amount: 2 },
		{ date: "2022-08-30", amount: 1 },
	],
};
export const MockDeploymentDAUResponse: TypesGen.DAUsResponse = {
	tz_hour_offset: 0,
	entries: [
		{ date: "2022-08-27", amount: 10 },
		{ date: "2022-08-29", amount: 22 },
		{ date: "2022-08-30", amount: 14 },
	],
};
export const MockSessionToken: TypesGen.LoginWithPasswordResponse = {
	session_token: "my-session-token",
};

export const MockAPIKey: TypesGen.GenerateAPIKeyResponse = {
	key: "my-api-key",
};

export const MockToken: TypesGen.APIKeyWithOwner = {
	id: "tBoVE3dqLl",
	user_id: "f9ee61d8-1d84-4410-ab6e-c1ec1a641e0b",
	last_used: "0001-01-01T00:00:00Z",
	expires_at: "2023-01-15T20:10:45.637438Z",
	created_at: "2022-12-16T20:10:45.637452Z",
	updated_at: "2022-12-16T20:10:45.637452Z",
	login_type: "token",
	scope: "all",
	lifetime_seconds: 2592000,
	token_name: "token-one",
	username: "admin",
};

export const MockTokens: TypesGen.APIKeyWithOwner[] = [
	MockToken,
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
		token_name: "token-two",
		username: "admin",
	},
];

export const MockPrimaryWorkspaceProxy: TypesGen.WorkspaceProxy = {
	id: "4aa23000-526a-481f-a007-0f20b98b1e12",
	name: "primary",
	display_name: "Default",
	icon_url: "/emojis/1f60e.png",
	healthy: true,
	path_app_url: "https://coder.com",
	wildcard_hostname: "*.coder.com",
	derp_enabled: true,
	derp_only: false,
	created_at: new Date().toISOString(),
	updated_at: new Date().toISOString(),
	version: "v2.34.5-test+primary",
	deleted: false,
	status: {
		status: "ok",
		checked_at: new Date().toISOString(),
	},
};

export const MockHealthyWildWorkspaceProxy: TypesGen.WorkspaceProxy = {
	id: "5e2c1ab7-479b-41a9-92ce-aa85625de52c",
	name: "haswildcard",
	display_name: "Subdomain Supported",
	icon_url: "/emojis/1f319.png",
	healthy: true,
	path_app_url: "https://external.com",
	wildcard_hostname: "*.external.com",
	derp_enabled: true,
	derp_only: false,
	created_at: new Date().toISOString(),
	updated_at: new Date().toISOString(),
	deleted: false,
	version: "v2.34.5-test+haswildcard",
	status: {
		status: "ok",
		checked_at: new Date().toISOString(),
	},
};

export const MockUnhealthyWildWorkspaceProxy: TypesGen.WorkspaceProxy = {
	id: "8444931c-0247-4171-842a-569d9f9cbadb",
	name: "unhealthy",
	display_name: "Unhealthy",
	icon_url: "/emojis/1f92e.png",
	healthy: false,
	path_app_url: "https://unhealthy.coder.com",
	wildcard_hostname: "*unhealthy..coder.com",
	derp_enabled: true,
	derp_only: true,
	created_at: new Date().toISOString(),
	updated_at: new Date().toISOString(),
	version: "v2.34.5-test+unhealthy",
	deleted: false,
	status: {
		status: "unhealthy",
		report: {
			errors: ["This workspace proxy is manually marked as unhealthy."],
			warnings: ["This is a manual warning for this workspace proxy."],
		},
		checked_at: new Date().toISOString(),
	},
};

export const MockWorkspaceProxies: TypesGen.WorkspaceProxy[] = [
	MockPrimaryWorkspaceProxy,
	MockHealthyWildWorkspaceProxy,
	MockUnhealthyWildWorkspaceProxy,
	{
		id: "26e84c16-db24-4636-a62d-aa1a4232b858",
		name: "nowildcard",
		display_name: "No wildcard",
		icon_url: "/emojis/1f920.png",
		healthy: true,
		path_app_url: "https://cowboy.coder.com",
		wildcard_hostname: "",
		derp_enabled: false,
		derp_only: false,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		deleted: false,
		version: "v2.34.5-test+nowildcard",
		status: {
			status: "ok",
			checked_at: new Date().toISOString(),
		},
	},
];

export const MockProxyLatencies: Record<string, ProxyLatencyReport> = {
	...MockWorkspaceProxies.reduce(
		(acc, proxy) => {
			if (!proxy.healthy) {
				return acc;
			}
			acc[proxy.id] = {
				// Make one of them inaccurate.
				accurate: proxy.id !== "26e84c16-db24-4636-a62d-aa1a4232b858",
				// This is a deterministic way to generate a latency to for each proxy.
				// It will be the same for each run as long as the IDs don't change.
				latencyMS:
					(Number(
						Array.from(proxy.id).reduce(
							// Multiply each char code by some large prime number to increase the
							// size of the number and allow use to get some decimal points.
							(acc, char) => acc + char.charCodeAt(0) * 37,
							0,
						),
					) /
						// Cap at 250ms
						100) %
					250,
				at: new Date(),
				nextHopProtocol:
					proxy.id === "8444931c-0247-4171-842a-569d9f9cbadb"
						? "http/1.1"
						: "h2",
			};
			return acc;
		},
		{} as Record<string, ProxyLatencyReport>,
	),
};

export const MockBuildInfo: TypesGen.BuildInfoResponse = {
	agent_api_version: "1.0",
	provisioner_api_version: "1.1",
	external_url: "file:///mock-url",
	version: "v2.99.99",
	dashboard_url: "https:///mock-url",
	workspace_proxy: false,
	upgrade_message: "My custom upgrade message",
	deployment_id: "510d407f-e521-4180-b559-eab4a6d802b8",
	telemetry: true,
};

export const MockSupportLinks: TypesGen.LinkConfig[] = [
	{
		name: "First link",
		target: "http://first-link",
		icon: "chat",
	},
	{
		name: "Second link",
		target: "http://second-link",
		icon: "docs",
	},
	{
		name: "Third link",
		target:
			"https://github.com/coder/coder/issues/new?labels=needs+grooming&body={CODER_BUILD_INFO}",
		icon: "",
	},
];

export const MockUpdateCheck: TypesGen.UpdateCheckResponse = {
	current: true,
	url: "file:///mock-url",
	version: "v99.999.9999+c9cdf14",
};

export const MockOwnerRole: TypesGen.Role = {
	name: "owner",
	display_name: "Owner",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: "",
};

export const MockUserAdminRole: TypesGen.Role = {
	name: "user_admin",
	display_name: "User Admin",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: "",
};

export const MockTemplateAdminRole: TypesGen.Role = {
	name: "template_admin",
	display_name: "Template Admin",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: "",
};

export const MockAuditorRole: TypesGen.Role = {
	name: "auditor",
	display_name: "Auditor",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: "",
};

export const MockMemberRole: TypesGen.SlimRole = {
	name: "member",
	display_name: "Member",
};

export const MockOrganizationAdminRole: TypesGen.Role = {
	name: "organization-admin",
	display_name: "Organization Admin",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: MockOrganization.id,
};

export const MockOrganizationUserAdminRole: TypesGen.Role = {
	name: "organization-user-admin",
	display_name: "Organization User Admin",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: MockOrganization.id,
};

export const MockOrganizationTemplateAdminRole: TypesGen.Role = {
	name: "organization-template-admin",
	display_name: "Organization Template Admin",
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: MockOrganization.id,
};

export const MockOrganizationAuditorRole: TypesGen.AssignableRoles = {
	name: "organization-auditor",
	display_name: "Organization Auditor",
	assignable: true,
	built_in: false,
	site_permissions: [],
	organization_permissions: [],
	user_permissions: [],
	organization_id: MockOrganization.id,
};

export const MockRoleWithOrgPermissions: TypesGen.AssignableRoles = {
	name: "my-role-1",
	display_name: "My Role 1",
	organization_id: MockOrganization.id,
	assignable: true,
	built_in: false,
	site_permissions: [],
	organization_permissions: [
		{
			negate: false,
			resource_type: "organization_member",
			action: "create",
		},
		{
			negate: false,
			resource_type: "organization_member",
			action: "delete",
		},
		{
			negate: false,
			resource_type: "organization_member",
			action: "read",
		},
		{
			negate: false,
			resource_type: "organization_member",
			action: "update",
		},
		{
			negate: false,
			resource_type: "template",
			action: "create",
		},
		{
			negate: false,
			resource_type: "template",
			action: "delete",
		},
		{
			negate: false,
			resource_type: "template",
			action: "read",
		},
		{
			negate: false,
			resource_type: "template",
			action: "update",
		},
		{
			negate: false,
			resource_type: "template",
			action: "view_insights",
		},
		{
			negate: false,
			resource_type: "audit_log",
			action: "create",
		},
		{
			negate: false,
			resource_type: "audit_log",
			action: "read",
		},
		{
			negate: false,
			resource_type: "group",
			action: "create",
		},
		{
			negate: false,
			resource_type: "group",
			action: "delete",
		},
		{
			negate: false,
			resource_type: "group",
			action: "read",
		},
		{
			negate: false,
			resource_type: "group",
			action: "update",
		},
		{
			negate: false,
			resource_type: "provisioner_daemon",
			action: "create",
		},
	],
	user_permissions: [],
};

export const MockRole2WithOrgPermissions: TypesGen.Role = {
	name: "my-role-1",
	display_name: "My Role 1",
	organization_id: MockOrganization.id,
	site_permissions: [],
	organization_permissions: [
		{
			negate: false,
			resource_type: "audit_log",
			action: "create",
		},
	],
	user_permissions: [],
};

// assignableRole takes a role and a boolean. The boolean implies if the
// actor can assign (add/remove) the role from other users.
export function assignableRole(
	role: TypesGen.Role,
	assignable: boolean,
): TypesGen.AssignableRoles {
	return {
		...role,
		assignable: assignable,
		built_in: true,
	};
}

export const MockSiteRoles = [MockUserAdminRole, MockAuditorRole];
export const MockAssignableSiteRoles = [
	assignableRole(MockUserAdminRole, true),
	assignableRole(MockAuditorRole, true),
];

export const MockMemberPermissions = {
	viewAuditLog: false,
};

export const MockUser: TypesGen.User = {
	id: "test-user",
	username: "TestUser",
	email: "test@coder.com",
	created_at: "",
	updated_at: "",
	status: "active",
	organization_ids: [MockOrganization.id],
	roles: [MockOwnerRole],
	avatar_url: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
	last_seen_at: "",
	login_type: "password",
	theme_preference: "",
	name: "",
};

export const MockUserAdmin: TypesGen.User = {
	...MockUser,
	roles: [MockUserAdminRole],
};

export const MockUser2: TypesGen.User = {
	id: "test-user-2",
	username: "TestUser2",
	email: "test2@coder.com",
	created_at: "",
	updated_at: "",
	status: "active",
	organization_ids: [MockOrganization.id],
	roles: [],
	avatar_url: "",
	last_seen_at: "2022-09-14T19:12:21Z",
	login_type: "oidc",
	theme_preference: "",
	name: "Mock User The Second",
};

export const SuspendedMockUser: TypesGen.User = {
	id: "suspended-mock-user",
	username: "SuspendedMockUser",
	email: "iamsuspendedsad!@coder.com",
	created_at: "",
	updated_at: "",
	status: "suspended",
	organization_ids: [MockOrganization.id],
	roles: [],
	avatar_url: "",
	last_seen_at: "",
	login_type: "password",
	theme_preference: "",
	name: "",
};

export const MockOrganizationMember: TypesGen.OrganizationMemberWithUserData = {
	organization_id: MockOrganization.id,
	user_id: MockUser.id,
	username: MockUser.username,
	email: MockUser.email,
	created_at: "",
	updated_at: "",
	name: MockUser.name,
	avatar_url: MockUser.avatar_url,
	global_roles: MockUser.roles,
	roles: [],
};

export const MockOrganizationMember2: TypesGen.OrganizationMemberWithUserData =
	{
		organization_id: MockOrganization.id,
		user_id: MockUser2.id,
		username: MockUser2.username,
		email: MockUser2.email,
		created_at: "",
		updated_at: "",
		name: MockUser2.name,
		avatar_url: MockUser2.avatar_url,
		global_roles: MockUser2.roles,
		roles: [],
	};

export const MockProvisionerKey: TypesGen.ProvisionerKey = {
	id: "test-provisioner-key",
	organization: MockOrganization.id,
	created_at: "2022-05-17T17:39:01.382927298Z",
	name: "test-name",
	tags: { scope: "organization" },
};

export const MockProvisionerBuiltinKey: TypesGen.ProvisionerKey = {
	...MockProvisionerKey,
	id: "00000000-0000-0000-0000-000000000001",
	name: "built-in",
};

export const MockProvisionerUserAuthKey: TypesGen.ProvisionerKey = {
	...MockProvisionerKey,
	id: "00000000-0000-0000-0000-000000000002",
	name: "user-auth",
};

export const MockProvisionerPskKey: TypesGen.ProvisionerKey = {
	...MockProvisionerKey,
	id: "00000000-0000-0000-0000-000000000003",
	name: "psk",
};

export const MockProvisioner: TypesGen.ProvisionerDaemon = {
	created_at: "2022-05-17T17:39:01.382927298Z",
	id: "test-provisioner",
	key_id: MockProvisionerBuiltinKey.id,
	organization_id: MockOrganization.id,
	name: "Test Provisioner",
	provisioners: ["echo"],
	tags: { scope: "organization" },
	version: MockBuildInfo.version,
	api_version: MockBuildInfo.provisioner_api_version,
	last_seen_at: new Date().toISOString(),
	key_name: "test-provisioner",
	status: "idle",
	current_job: null,
	previous_job: null,
};

export const MockUserAuthProvisioner: TypesGen.ProvisionerDaemon = {
	...MockProvisioner,
	id: "test-user-auth-provisioner",
	key_id: MockProvisionerUserAuthKey.id,
	name: `${MockUser.name}'s provisioner`,
	tags: { scope: "user" },
};

export const MockPskProvisioner: TypesGen.ProvisionerDaemon = {
	...MockProvisioner,
	id: "test-psk-provisioner",
	key_id: MockProvisionerPskKey.id,
	key_name: MockProvisionerPskKey.name,
	name: "Test psk provisioner",
};

export const MockKeyProvisioner: TypesGen.ProvisionerDaemon = {
	...MockProvisioner,
	id: "test-key-provisioner",
	key_id: MockProvisionerKey.id,
	key_name: MockProvisionerKey.name,
	organization_id: MockProvisionerKey.organization,
	name: "Test key provisioner",
	tags: MockProvisionerKey.tags,
};

export const MockProvisioner2: TypesGen.ProvisionerDaemon = {
	...MockProvisioner,
	id: "test-provisioner-2",
	name: "Test Provisioner 2",
	key_id: MockProvisionerKey.id,
	key_name: MockProvisionerKey.name,
};

export const MockUserProvisioner: TypesGen.ProvisionerDaemon = {
	...MockUserAuthProvisioner,
	id: "test-user-provisioner",
	name: "Test User Provisioner",
	tags: { scope: "user", owner: "12345678-abcd-1234-abcd-1234567890abcd" },
};

export const MockProvisionerWithTags: TypesGen.ProvisionerDaemon = {
	...MockProvisioner,
	id: "test-provisioner-tags",
	name: "Test Provisioner with tags",
	tags: {
		...MockProvisioner.tags,
		都市: "ユタ",
		きっぷ: "yes",
		ちいさい: "no",
	},
};

export const MockProvisionerJob: TypesGen.ProvisionerJob = {
	created_at: "",
	id: "test-provisioner-job",
	status: "succeeded",
	file_id: MockOrganization.id,
	completed_at: "2022-05-17T17:39:01.382927298Z",
	tags: {
		scope: "organization",
		owner: "",
		wowzers: "whatatag",
		isCapable: "false",
		department: "engineering",
		dreaming: "true",
	},
	queue_position: 0,
	queue_size: 0,
	input: {
		template_version_id: "test-template-version", // MockTemplateVersion.id
	},
	organization_id: MockOrganization.id,
	type: "template_version_dry_run",
	metadata: {
		workspace_id: "test-workspace",
		template_display_name: "Test Template",
		template_icon: "/icon/code.svg",
		template_id: "test-template",
		template_name: "test-template",
		template_version_name: "test-version",
		workspace_name: "test-workspace",
	},
};

export const MockFailedProvisionerJob: TypesGen.ProvisionerJob = {
	...MockProvisionerJob,
	status: "failed",
};

export const MockCancelingProvisionerJob: TypesGen.ProvisionerJob = {
	...MockProvisionerJob,
	status: "canceling",
};
export const MockCanceledProvisionerJob: TypesGen.ProvisionerJob = {
	...MockProvisionerJob,
	status: "canceled",
};
export const MockRunningProvisionerJob: TypesGen.ProvisionerJob = {
	...MockProvisionerJob,
	status: "running",
};
export const MockPendingProvisionerJob: TypesGen.ProvisionerJob = {
	...MockProvisionerJob,
	status: "pending",
	queue_position: 2,
	queue_size: 4,
};
export const MockTemplateVersion: TypesGen.TemplateVersion = {
	id: "test-template-version",
	created_at: "2022-05-17T17:39:01.382927298Z",
	updated_at: "2022-05-17T17:39:01.382927298Z",
	template_id: "test-template",
	job: MockProvisionerJob,
	name: "test-version",
	message: "first version",
	readme: `---
name:Template test
---
## Instructions
You can add instructions here

[Some link info](https://coder.com)`,
	created_by: MockUser,
	archived: false,
};

export const MockTemplateVersion2: TypesGen.TemplateVersion = {
	id: "test-template-version-2",
	created_at: "2022-05-17T17:39:01.382927298Z",
	updated_at: "2022-05-17T17:39:01.382927298Z",
	template_id: "test-template",
	job: MockProvisionerJob,
	name: "test-version-2",
	message: "first version",
	readme: `---
name:Template test 2
---
## Instructions
You can add instructions here

[Some link info](https://coder.com)`,
	created_by: MockUser,
	archived: false,
};

export const MockTemplateVersionWithMarkdownMessage: TypesGen.TemplateVersion =
	{
		...MockTemplateVersion,
		message: `
# Abiding Grace
## Enchantment
At the beginning of your end step, choose one —

- You gain 1 life.

- Return target creature card with mana value 1 from your graveyard to the battlefield.
`,
	};

export const MockTemplate: TypesGen.Template = {
	id: "test-template",
	created_at: "2022-05-17T17:39:01.382927298Z",
	updated_at: "2022-05-18T17:39:01.382927298Z",
	organization_id: MockOrganization.id,
	organization_name: MockOrganization.name,
	organization_display_name: MockOrganization.display_name,
	organization_icon: "/emojis/1f5fa.png",
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
	activity_bump_ms: 1 * 60 * 60 * 1000,
	autostop_requirement: {
		days_of_week: ["sunday"],
		weeks: 1,
	},
	autostart_requirement: {
		days_of_week: [
			"monday",
			"tuesday",
			"wednesday",
			"thursday",
			"friday",
			"saturday",
			"sunday",
		],
	},
	created_by_id: "test-creator-id",
	created_by_name: "test_creator",
	icon: "/icon/code.svg",
	allow_user_cancel_workspace_jobs: true,
	failure_ttl_ms: 0,
	time_til_dormant_ms: 0,
	time_til_dormant_autodelete_ms: 0,
	allow_user_autostart: true,
	allow_user_autostop: true,
	require_active_version: false,
	deprecated: false,
	deprecation_message: "",
	max_port_share_level: "public",
};

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
};

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
};

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
	hidden: false,
	open_in: "slim-window",
};

export const MockWorkspaceAgentLogSource: TypesGen.WorkspaceAgentLogSource = {
	created_at: "2023-05-04T11:30:41.402072Z",
	id: "dc790496-eaec-4f88-a53f-8ce1f61a1fff",
	display_name: "Startup Script",
	icon: "",
	workspace_agent_id: "",
};

export const MockWorkspaceAgentScript: TypesGen.WorkspaceAgentScript = {
	id: "08eaca83-1221-4fad-b882-d1136981f54d",
	log_source_id: MockWorkspaceAgentLogSource.id,
	cron: "",
	log_path: "",
	run_on_start: true,
	run_on_stop: false,
	script: "echo 'hello world'",
	start_blocks_login: false,
	timeout: 0,
	display_name: "Say Hello",
};

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
	api_version: MockBuildInfo.agent_api_version,
	latency: {
		"Coder Embedded DERP": {
			latency_ms: 32.55,
			preferred: true,
		},
	},
	connection_timeout_seconds: 120,
	troubleshooting_url: "https://coder.com/troubleshoot",
	lifecycle_state: "starting",
	logs_length: 0,
	logs_overflowed: false,
	log_sources: [MockWorkspaceAgentLogSource],
	scripts: [MockWorkspaceAgentScript],
	startup_script_behavior: "non-blocking",
	subsystems: ["envbox", "exectrace"],
	health: {
		healthy: true,
	},
	display_apps: [
		"ssh_helper",
		"port_forwarding_helper",
		"vscode",
		"vscode_insiders",
		"web_terminal",
	],
};

export const MockWorkspaceAgentDisconnected: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-2",
	name: "another-workspace-agent",
	status: "disconnected",
	version: "",
	latency: {},
	lifecycle_state: "ready",
	health: {
		healthy: false,
		reason: "agent is not connected",
	},
};

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
};

export const MockWorkspaceAgentDeprecated: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-3",
	name: "an-outdated-workspace-agent",
	version: "v99.999.9998+abcdef",
	api_version: "1.99",
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
};

export const MockWorkspaceAgentConnecting: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-connecting",
	name: "another-workspace-agent",
	status: "connecting",
	version: "",
	latency: {},
	lifecycle_state: "created",
};

export const MockWorkspaceAgentTimeout: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-timeout",
	name: "a-timed-out-workspace-agent",
	status: "timeout",
	version: "",
	latency: {},
	lifecycle_state: "created",
	health: {
		healthy: false,
		reason: "agent is taking too long to connect",
	},
};

export const MockWorkspaceAgentStarting: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-starting",
	name: "a-starting-workspace-agent",
	lifecycle_state: "starting",
};

export const MockWorkspaceAgentReady: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-ready",
	name: "a-ready-workspace-agent",
	lifecycle_state: "ready",
};

export const MockWorkspaceAgentStartTimeout: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-start-timeout",
	name: "a-workspace-agent-timed-out-while-running-startup-script",
	lifecycle_state: "start_timeout",
};

export const MockWorkspaceAgentStartError: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-start-error",
	name: "a-workspace-agent-errored-while-running-startup-script",
	lifecycle_state: "start_error",
	health: {
		healthy: false,
		reason: "agent startup script failed",
	},
};

export const MockWorkspaceAgentShuttingDown: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-shutting-down",
	name: "a-shutting-down-workspace-agent",
	lifecycle_state: "shutting_down",
	health: {
		healthy: false,
		reason: "agent is shutting down",
	},
};

export const MockWorkspaceAgentShutdownTimeout: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-shutdown-timeout",
	name: "a-workspace-agent-timed-out-while-running-shutdownup-script",
	lifecycle_state: "shutdown_timeout",
	health: {
		healthy: false,
		reason: "agent is shutting down",
	},
};

export const MockWorkspaceAgentShutdownError: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-shutdown-error",
	name: "a-workspace-agent-errored-while-running-shutdownup-script",
	lifecycle_state: "shutdown_error",
	health: {
		healthy: false,
		reason: "agent is shutting down",
	},
};

export const MockWorkspaceAgentOff: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "test-workspace-agent-off",
	name: "a-workspace-agent-is-shut-down",
	lifecycle_state: "off",
	health: {
		healthy: false,
		reason: "agent is shutting down",
	},
};

export const MockWorkspaceResource: TypesGen.WorkspaceResource = {
	id: "test-workspace-resource",
	name: "a-workspace-resource",
	agents: [MockWorkspaceAgent],
	created_at: "",
	job_id: "",
	type: "google_compute_disk",
	workspace_transition: "start",
	hide: false,
	icon: "",
	metadata: [{ key: "size", value: "32GB", sensitive: false }],
	daily_cost: 10,
};

export const MockWorkspaceResourceSensitive: TypesGen.WorkspaceResource = {
	...MockWorkspaceResource,
	id: "test-workspace-resource-sensitive",
	name: "workspace-resource-sensitive",
	metadata: [{ key: "api_key", value: "12345678", sensitive: true }],
};

export const MockWorkspaceResourceMultipleAgents: TypesGen.WorkspaceResource = {
	...MockWorkspaceResource,
	id: "test-workspace-resource-multiple-agents",
	name: "workspace-resource-multiple-agents",
	agents: [
		MockWorkspaceAgent,
		MockWorkspaceAgentDisconnected,
		MockWorkspaceAgentOutdated,
	],
};

export const MockWorkspaceResourceHidden: TypesGen.WorkspaceResource = {
	...MockWorkspaceResource,
	id: "test-workspace-resource-hidden",
	name: "workspace-resource-hidden",
	hide: true,
};

export const MockWorkspaceVolumeResource: TypesGen.WorkspaceResource = {
	id: "test-workspace-volume-resource",
	created_at: "",
	job_id: "",
	workspace_transition: "start",
	type: "docker_volume",
	name: "home_volume",
	hide: false,
	icon: "",
	daily_cost: 0,
};

export const MockWorkspaceImageResource: TypesGen.WorkspaceResource = {
	id: "test-workspace-image-resource",
	created_at: "",
	job_id: "",
	workspace_transition: "start",
	type: "docker_image",
	name: "main",
	hide: false,
	icon: "",
	daily_cost: 0,
};

export const MockWorkspaceContainerResource: TypesGen.WorkspaceResource = {
	id: "test-workspace-container-resource",
	created_at: "",
	job_id: "",
	workspace_transition: "start",
	type: "docker_container",
	name: "workspace",
	hide: false,
	icon: "",
	daily_cost: 0,
};

export const MockWorkspaceAutostartDisabled: TypesGen.UpdateWorkspaceAutostartRequest =
	{
		schedule: "",
	};

export const MockWorkspaceAutostartEnabled: TypesGen.UpdateWorkspaceAutostartRequest =
	{
		// Runs at 9:30am Monday through Friday using Canada/Eastern
		// (America/Toronto) time
		schedule: "CRON_TZ=Canada/Eastern 30 9 * * 1-5",
	};

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
	workspace_owner_avatar_url: MockUser.avatar_url,
	workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
	deadline: "2022-05-17T23:39:00.00Z",
	reason: "initiator",
	resources: [MockWorkspaceResource],
	status: "running",
	daily_cost: 20,
	matched_provisioners: {
		count: 1,
		available: 1,
	},
};

export const MockWorkspaceBuildAutostart: TypesGen.WorkspaceBuild = {
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
	workspace_owner_avatar_url: MockUser.avatar_url,
	workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
	deadline: "2022-05-17T23:39:00.00Z",
	reason: "autostart",
	resources: [MockWorkspaceResource],
	status: "running",
	daily_cost: 20,
};

export const MockWorkspaceBuildAutostop: TypesGen.WorkspaceBuild = {
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
	workspace_owner_avatar_url: MockUser.avatar_url,
	workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
	deadline: "2022-05-17T23:39:00.00Z",
	reason: "autostop",
	resources: [MockWorkspaceResource],
	status: "running",
	daily_cost: 20,
};

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
	workspace_owner_avatar_url: MockUser.avatar_url,
	workspace_id: "759f1d46-3174-453d-aa60-980a9c1442f3",
	deadline: "2022-05-17T23:39:00.00Z",
	reason: "initiator",
	resources: [],
	status: "failed",
	daily_cost: 20,
});

export const MockWorkspaceBuildStop: TypesGen.WorkspaceBuild = {
	...MockWorkspaceBuild,
	id: "2",
	transition: "stop",
};

export const MockWorkspaceBuildDelete: TypesGen.WorkspaceBuild = {
	...MockWorkspaceBuild,
	id: "3",
	transition: "delete",
};

export const MockBuilds = [
	{ ...MockWorkspaceBuild, id: "1" },
	{ ...MockWorkspaceBuildAutostart, id: "2" },
	{ ...MockWorkspaceBuildAutostop, id: "3" },
	{ ...MockWorkspaceBuildStop, id: "4" },
	{ ...MockWorkspaceBuildDelete, id: "5" },
];

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
	template_active_version_id: MockTemplate.active_version_id,
	template_require_active_version: MockTemplate.require_active_version,
	outdated: false,
	owner_id: MockUser.id,
	organization_id: MockOrganization.id,
	organization_name: "default",
	owner_name: MockUser.username,
	owner_avatar_url: "https://avatars.githubusercontent.com/u/7122116?v=4",
	autostart_schedule: MockWorkspaceAutostartEnabled.schedule,
	ttl_ms: 2 * 60 * 60 * 1000,
	latest_build: MockWorkspaceBuild,
	last_used_at: "2022-05-16T15:29:10.302441433Z",
	health: {
		healthy: true,
		failing_agents: [],
	},
	automatic_updates: "never",
	allow_renames: true,
	favorite: false,
	deleting_at: null,
	dormant_at: null,
	next_start_at: null,
};

export const MockFavoriteWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-favorite-workspace",
	favorite: true,
};

export const MockStoppedWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-stopped-workspace",
	latest_build: { ...MockWorkspaceBuildStop, status: "stopped" },
};
export const MockStoppingWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-stopping-workspace",
	latest_build: {
		...MockWorkspaceBuildStop,
		job: MockRunningProvisionerJob,
		status: "stopping",
	},
};
export const MockStartingWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-starting-workspace",
	latest_build: {
		...MockWorkspaceBuild,
		job: MockRunningProvisionerJob,
		transition: "start",
		status: "starting",
	},
};
export const MockCancelingWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-canceling-workspace",
	latest_build: {
		...MockWorkspaceBuild,
		job: MockCancelingProvisionerJob,
		status: "canceling",
	},
};
export const MockCanceledWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-canceled-workspace",
	latest_build: {
		...MockWorkspaceBuild,
		job: MockCanceledProvisionerJob,
		status: "canceled",
	},
};
export const MockFailedWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-failed-workspace",
	latest_build: {
		...MockWorkspaceBuild,
		job: MockFailedProvisionerJob,
		status: "failed",
	},
};
export const MockDeletingWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-deleting-workspace",
	latest_build: {
		...MockWorkspaceBuildDelete,
		job: MockRunningProvisionerJob,
		status: "deleting",
	},
};

export const MockWorkspaceWithDeletion = {
	...MockStoppedWorkspace,
	deleting_at: new Date().toISOString(),
};

export const MockDeletedWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-deleted-workspace",
	latest_build: { ...MockWorkspaceBuildDelete, status: "deleted" },
};

export const MockOutdatedWorkspace: TypesGen.Workspace = {
	...MockFailedWorkspace,
	id: "test-outdated-workspace",
	outdated: true,
};

export const MockRunningOutdatedWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-running-outdated-workspace",
	outdated: true,
};

export const MockDormantWorkspace: TypesGen.Workspace = {
	...MockStoppedWorkspace,
	id: "test-dormant-workspace",
	dormant_at: new Date().toISOString(),
};

export const MockDormantOutdatedWorkspace: TypesGen.Workspace = {
	...MockStoppedWorkspace,
	id: "test-dormant-outdated-workspace",
	name: "Dormant-Workspace",
	outdated: true,
	dormant_at: new Date().toISOString(),
};

export const MockOutdatedRunningWorkspaceRequireActiveVersion: TypesGen.Workspace =
	{
		...MockWorkspace,
		id: "test-outdated-workspace-require-active-version",
		outdated: true,
		template_require_active_version: true,
	};

export const MockOutdatedRunningWorkspaceAlwaysUpdate: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-outdated-workspace-always-update",
	outdated: true,
	automatic_updates: "always",
	latest_build: {
		...MockWorkspaceBuild,
		status: "running",
	},
};

export const MockOutdatedStoppedWorkspaceRequireActiveVersion: TypesGen.Workspace =
	{
		...MockOutdatedRunningWorkspaceRequireActiveVersion,
		latest_build: {
			...MockWorkspaceBuild,
			status: "stopped",
		},
	};

export const MockOutdatedStoppedWorkspaceAlwaysUpdate: TypesGen.Workspace = {
	...MockOutdatedRunningWorkspaceAlwaysUpdate,
	latest_build: {
		...MockWorkspaceBuild,
		status: "stopped",
	},
};

export const MockPendingWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "test-pending-workspace",
	latest_build: {
		...MockWorkspaceBuild,
		job: MockPendingProvisionerJob,
		transition: "start",
		status: "pending",
	},
};

// just over one page of workspaces
export const MockWorkspacesResponse: TypesGen.WorkspacesResponse = {
	workspaces: range(1, 27).map((id: number) => ({
		...MockWorkspace,
		id: id.toString(),
		name: `${MockWorkspace.name}${id}`,
	})),
	count: 26,
};

export const MockWorkspacesResponseWithDeletions = {
	workspaces: [...MockWorkspacesResponse.workspaces, MockWorkspaceWithDeletion],
	count: MockWorkspacesResponse.count + 1,
};

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
		required: true,
		ephemeral: false,
	};

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
		required: true,
		ephemeral: false,
	};

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
		required: true,
		ephemeral: false,
	};

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
		required: true,
		ephemeral: false,
	};

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
		required: true,
		ephemeral: false,
	};

export const MockTemplateVersionVariable1: TypesGen.TemplateVersionVariable = {
	name: "first_variable",
	description: "This is first variable.",
	type: "string",
	value: "",
	default_value: "abc",
	required: false,
	sensitive: false,
};

export const MockTemplateVersionVariable2: TypesGen.TemplateVersionVariable = {
	name: "second_variable",
	description: "This is second variable.",
	type: "number",
	value: "5",
	default_value: "3",
	required: false,
	sensitive: false,
};

export const MockTemplateVersionVariable3: TypesGen.TemplateVersionVariable = {
	name: "third_variable",
	description: "This is third variable.",
	type: "bool",
	value: "",
	default_value: "false",
	required: false,
	sensitive: false,
};

export const MockTemplateVersionVariable4: TypesGen.TemplateVersionVariable = {
	name: "fourth_variable",
	description: "This is fourth variable.",
	type: "string",
	value: "defghijk",
	default_value: "",
	required: true,
	sensitive: true,
};

export const MockTemplateVersionVariable5: TypesGen.TemplateVersionVariable = {
	name: "fifth_variable",
	description: "This is fifth variable.",
	type: "string",
	value: "",
	default_value: "",
	required: true,
	sensitive: false,
};

export const MockWorkspaceRequest: TypesGen.CreateWorkspaceRequest = {
	name: "test",
	template_version_id: "test-template-version",
	rich_parameter_values: [],
};

export const MockWorkspaceRichParametersRequest: TypesGen.CreateWorkspaceRequest =
	{
		name: "test",
		template_version_id: "test-template-version",
		rich_parameter_values: [
			{
				name: MockTemplateVersionParameter1.name,
				value: MockTemplateVersionParameter1.default_value,
			},
		],
	};

export const MockUserAgent = {
	browser: "Chrome 99.0.4844",
	device: "Other",
	ip_address: "11.22.33.44",
	os: "Windows 10",
};

export const MockAuthMethodsPasswordOnly: TypesGen.AuthMethods = {
	password: { enabled: true },
	github: { enabled: false },
	oidc: { enabled: false, signInText: "", iconUrl: "" },
};

export const MockAuthMethodsPasswordTermsOfService: TypesGen.AuthMethods = {
	terms_of_service_url: "https://www.youtube.com/watch?v=C2f37Vb2NAE",
	password: { enabled: true },
	github: { enabled: false },
	oidc: { enabled: false, signInText: "", iconUrl: "" },
};

export const MockAuthMethodsExternal: TypesGen.AuthMethods = {
	password: { enabled: false },
	github: { enabled: true },
	oidc: {
		enabled: true,
		signInText: "Google",
		iconUrl: "/icon/google.svg",
	},
};

export const MockAuthMethodsAll: TypesGen.AuthMethods = {
	password: { enabled: true },
	github: { enabled: true },
	oidc: {
		enabled: true,
		signInText: "Google",
		iconUrl: "/icon/google.svg",
	},
};

export const MockGitSSHKey: TypesGen.GitSSHKey = {
	user_id: "1fa0200f-7331-4524-a364-35770666caa7",
	created_at: "2022-05-16T14:30:34.148205897Z",
	updated_at: "2022-05-16T15:29:10.302441433Z",
	public_key:
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFJOQRIM7kE30rOzrfy+/+R+nQGCk7S9pioihy+2ARbq",
};

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
];

export const MockWorkspaceExtendedBuildLogs: TypesGen.ProvisionerJobLog[] = [
	{
		id: 938494,
		created_at: "2023-08-25T19:07:43.331Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Setting up",
		output: "",
	},
	{
		id: 938495,
		created_at: "2023-08-25T19:07:43.331Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Parsing template parameters",
		output: "",
	},
	{
		id: 938496,
		created_at: "2023-08-25T19:07:43.339Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Detecting persistent resources",
		output: "",
	},
	{
		id: 938497,
		created_at: "2023-08-25T19:07:44.15Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "Initializing the backend...",
	},
	{
		id: 938498,
		created_at: "2023-08-25T19:07:44.215Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "Initializing provider plugins...",
	},
	{
		id: 938499,
		created_at: "2023-08-25T19:07:44.216Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: '- Finding coder/coder versions matching "~> 0.11.0"...',
	},
	{
		id: 938500,
		created_at: "2023-08-25T19:07:44.668Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: '- Finding kreuzwerker/docker versions matching "~> 3.0.1"...',
	},
	{
		id: 938501,
		created_at: "2023-08-25T19:07:44.722Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "- Using coder/coder v0.11.1 from the shared cache directory",
	},
	{
		id: 938502,
		created_at: "2023-08-25T19:07:44.857Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "- Using kreuzwerker/docker v3.0.2 from the shared cache directory",
	},
	{
		id: 938503,
		created_at: "2023-08-25T19:07:45.081Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"Terraform has created a lock file .terraform.lock.hcl to record the provider",
	},
	{
		id: 938504,
		created_at: "2023-08-25T19:07:45.081Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"selections it made above. Include this file in your version control repository",
	},
	{
		id: 938505,
		created_at: "2023-08-25T19:07:45.081Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"so that Terraform can guarantee to make the same selections by default when",
	},
	{
		id: 938506,
		created_at: "2023-08-25T19:07:45.082Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: 'you run "terraform init" in the future.',
	},
	{
		id: 938507,
		created_at: "2023-08-25T19:07:45.083Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "Terraform has been successfully initialized!",
	},
	{
		id: 938508,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			'You may now begin working with Terraform. Try running "terraform plan" to see',
	},
	{
		id: 938509,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"any changes that are required for your infrastructure. All Terraform commands",
	},
	{
		id: 938510,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "should now work.",
	},
	{
		id: 938511,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"If you ever set or change modules or backend configuration for Terraform,",
	},
	{
		id: 938512,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output:
			"rerun this command to reinitialize your working directory. If you forget, other",
	},
	{
		id: 938513,
		created_at: "2023-08-25T19:07:45.084Z",
		log_source: "provisioner",
		log_level: "debug",
		stage: "Detecting persistent resources",
		output: "commands will detect it and remind you to do so if necessary.",
	},
	{
		id: 938514,
		created_at: "2023-08-25T19:07:45.143Z",
		log_source: "provisioner",
		log_level: "info",
		stage: "Detecting persistent resources",
		output: "Terraform 1.1.9",
	},
	{
		id: 938515,
		created_at: "2023-08-25T19:07:46.297Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "Warning: Argument is deprecated",
	},
	{
		id: 938516,
		created_at: "2023-08-25T19:07:46.297Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: 'on devcontainer-on-docker.tf line 15, in provider "coder":',
	},
	{
		id: 938517,
		created_at: "2023-08-25T19:07:46.297Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "  15:   feature_use_managed_variables = true",
	},
	{
		id: 938518,
		created_at: "2023-08-25T19:07:46.297Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "",
	},
	{
		id: 938519,
		created_at: "2023-08-25T19:07:46.297Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output:
			"Terraform variables are now exclusively utilized for template-wide variables after the removal of support for legacy parameters.",
	},
	{
		id: 938520,
		created_at: "2023-08-25T19:07:46.3Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Detecting persistent resources",
		output: "Error: ephemeral parameter requires the default property",
	},
	{
		id: 938521,
		created_at: "2023-08-25T19:07:46.3Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Detecting persistent resources",
		output:
			'on devcontainer-on-docker.tf line 27, in data "coder_parameter" "another_one":',
	},
	{
		id: 938522,
		created_at: "2023-08-25T19:07:46.3Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Detecting persistent resources",
		output: '  27: data "coder_parameter" "another_one" {',
	},
	{
		id: 938523,
		created_at: "2023-08-25T19:07:46.301Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Detecting persistent resources",
		output: "",
	},
	{
		id: 938524,
		created_at: "2023-08-25T19:07:46.301Z",
		log_source: "provisioner",
		log_level: "error",
		stage: "Detecting persistent resources",
		output: "",
	},
	{
		id: 938525,
		created_at: "2023-08-25T19:07:46.303Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "Warning: Argument is deprecated",
	},
	{
		id: 938526,
		created_at: "2023-08-25T19:07:46.303Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: 'on devcontainer-on-docker.tf line 15, in provider "coder":',
	},
	{
		id: 938527,
		created_at: "2023-08-25T19:07:46.303Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "  15:   feature_use_managed_variables = true",
	},
	{
		id: 938528,
		created_at: "2023-08-25T19:07:46.303Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output: "",
	},
	{
		id: 938529,
		created_at: "2023-08-25T19:07:46.303Z",
		log_source: "provisioner",
		log_level: "warn",
		stage: "Detecting persistent resources",
		output:
			"Terraform variables are now exclusively utilized for template-wide variables after the removal of support for legacy parameters.",
	},
	{
		id: 938530,
		created_at: "2023-08-25T19:07:46.311Z",
		log_source: "provisioner_daemon",
		log_level: "info",
		stage: "Cleaning Up",
		output: "",
	},
];

export const MockCancellationMessage = {
	message: "Job successfully canceled",
};

type MockAPIInput = {
	message?: string;
	detail?: string;
	validations?: FieldError[];
};

type MockAPIOutput = {
	isAxiosError: true;
	response: {
		data: {
			message: string;
			detail: string | undefined;
			validations: FieldError[] | undefined;
		};
	};
};

export const mockApiError = ({
	message = "Something went wrong.",
	detail,
	validations,
}: MockAPIInput): MockAPIOutput => ({
	// This is how axios can check if it is an axios error when calling isAxiosError
	isAxiosError: true,
	response: {
		data: {
			message,
			detail,
			validations,
		},
	},
});

export const MockEntitlements: TypesGen.Entitlements = {
	errors: [],
	warnings: [],
	has_license: false,
	features: withDefaultFeatures({
		workspace_batch_actions: {
			enabled: true,
			entitlement: "entitled",
		},
	}),
	require_telemetry: false,
	trial: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
};

export const MockEntitlementsWithWarnings: TypesGen.Entitlements = {
	errors: [],
	warnings: ["You are over your active user limit.", "And another thing."],
	has_license: true,
	trial: false,
	require_telemetry: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
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
};

export const MockEntitlementsWithAuditLog: TypesGen.Entitlements = {
	errors: [],
	warnings: [],
	has_license: true,
	require_telemetry: false,
	trial: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
	features: withDefaultFeatures({
		audit_log: {
			enabled: true,
			entitlement: "entitled",
		},
	}),
};

export const MockEntitlementsWithScheduling: TypesGen.Entitlements = {
	errors: [],
	warnings: [],
	has_license: true,
	require_telemetry: false,
	trial: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
	features: withDefaultFeatures({
		advanced_template_scheduling: {
			enabled: true,
			entitlement: "entitled",
		},
	}),
};

export const MockEntitlementsWithUserLimit: TypesGen.Entitlements = {
	errors: [],
	warnings: [],
	has_license: true,
	require_telemetry: false,
	trial: false,
	refreshed_at: "2022-05-20T16:45:57.122Z",
	features: withDefaultFeatures({
		user_limit: {
			enabled: true,
			entitlement: "entitled",
			limit: 25,
		},
	}),
};

export const MockEntitlementsWithMultiOrg: TypesGen.Entitlements = {
	...MockEntitlements,
	has_license: true,
	features: withDefaultFeatures({
		multiple_organizations: {
			enabled: true,
			entitlement: "entitled",
		},
	}),
};

export const MockExperiments: TypesGen.Experiment[] = [];

/**
 * An audit log for MockOrganization.
 */
export const MockAuditLog: TypesGen.AuditLog = {
	id: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
	request_id: "53bded77-7b9d-4e82-8771-991a34d759f9",
	time: "2022-05-19T16:45:57.122Z",
	organization_id: MockOrganization.id,
	organization: {
		id: MockOrganization.id,
		name: MockOrganization.name,
		display_name: MockOrganization.display_name,
		icon: MockOrganization.icon,
	},
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
};

/**
 * An audit log for MockOrganization2.
 */
export const MockAuditLog2: TypesGen.AuditLog = {
	...MockAuditLog,
	id: "53bded77-7b9d-4e82-8771-991a34d759f9",
	action: "write",
	time: "2022-05-20T16:45:57.122Z",
	description: "{user} updated workspace {target}",
	organization_id: MockOrganization2.id,
	organization: {
		id: MockOrganization2.id,
		name: MockOrganization2.name,
		display_name: MockOrganization2.display_name,
		icon: MockOrganization2.icon,
	},
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
};

/**
 * An audit log without an organization.
 */
export const MockAuditLog3: TypesGen.AuditLog = {
	id: "8efa9208-656a-422d-842d-b9dec0cf1bf3",
	request_id: "57ee9510-8330-480d-9ffa-4024e5805465",
	time: "2024-06-11T01:32:11.123Z",
	organization_id: "00000000-0000-0000-000000000000",
	ip: "127.0.0.1",
	user_agent:
		'"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36"',
	resource_type: "template",
	resource_id: "a624458c-1562-4689-a671-42c0b7d2d0c5",
	resource_target: "docker",
	resource_icon: "",
	action: "write",
	diff: {
		display_name: {
			old: "old display",
			new: "new display",
			secret: false,
		},
	},
	status_code: 200,
	additional_fields: {},
	description: "{user} updated template {target}",
	user: MockUser,
	resource_link: "/templates/docker",
	is_deleted: false,
};

export const MockWorkspaceCreateAuditLogForDifferentOwner = {
	...MockAuditLog,
	additional_fields: {
		workspace_owner: "Member",
	},
};

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
};

export const MockAuditLogWithDeletedResource: TypesGen.AuditLog = {
	...MockAuditLog,
	is_deleted: true,
};

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
};

export const MockAuditOauthConvert: TypesGen.AuditLog = {
	...MockAuditLog,
	resource_type: "convert_login",
	resource_target: "oidc",
	action: "create",
	status_code: 201,
	description: "{user} created login type conversion to {target}}",
	diff: {
		created_at: {
			old: "0001-01-01T00:00:00Z",
			new: "2023-06-20T20:44:54.243019Z",
			secret: false,
		},
		expires_at: {
			old: "0001-01-01T00:00:00Z",
			new: "2023-06-20T20:49:54.243019Z",
			secret: false,
		},
		state_string: {
			old: "",
			new: "",
			secret: true,
		},
		to_type: {
			old: "",
			new: "oidc",
			secret: false,
		},
		user_id: {
			old: "",
			new: "dc790496-eaec-4f88-a53f-8ce1f61a1fff",
			secret: false,
		},
	},
};

export const MockAuditLogSuccessfulLogin: TypesGen.AuditLog = {
	...MockAuditLog,
	resource_type: "api_key",
	resource_target: "",
	action: "login",
	status_code: 201,
	description: "{user} logged in",
};

export const MockAuditLogUnsuccessfulLoginKnownUser: TypesGen.AuditLog = {
	...MockAuditLogSuccessfulLogin,
	status_code: 401,
};

export const MockAuditLogRequestPasswordReset: TypesGen.AuditLog = {
	...MockAuditLog,
	resource_type: "user",
	resource_target: "member",
	action: "request_password_reset",
	description: "password reset requested for {target}",
	diff: {
		hashed_password: {
			old: "",
			new: "",
			secret: true,
		},
		one_time_passcode_expires_at: {
			old: {
				Time: "0001-01-01T00:00:00Z",
				Valid: false,
			},
			new: {
				Time: "2024-10-22T09:03:23.961702Z",
				Valid: true,
			},
			secret: false,
		},
	},
};

export const MockWorkspaceQuota: TypesGen.WorkspaceQuota = {
	credits_consumed: 0,
	budget: 100,
};

export const MockGroupSyncSettings: TypesGen.GroupSyncSettings = {
	field: "group-test",
	mapping: {
		"idp-group-1": [
			"fbd2116a-8961-4954-87ae-e4575bd29ce0",
			"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
		],
		"idp-group-2": ["fbd2116a-8961-4954-87ae-e4575bd29ce0"],
	},
	regex_filter: "@[a-zA-Z0-9_]+",
	auto_create_missing_groups: false,
};

export const MockLegacyMappingGroupSyncSettings = {
	...MockGroupSyncSettings,
	mapping: {},
	legacy_group_name_mapping: {
		"idp-group-1": "fbd2116a-8961-4954-87ae-e4575bd29ce0",
		"idp-group-2": "13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
	},
} satisfies TypesGen.GroupSyncSettings;

export const MockGroupSyncSettings2: TypesGen.GroupSyncSettings = {
	field: "group-test",
	mapping: {
		"idp-group-1": [
			"fbd2116a-8961-4954-87ae-e4575bd29ce0",
			"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e3",
		],
		"idp-group-2": ["fbd2116a-8961-4954-87ae-e4575bd29ce2"],
	},
	regex_filter: "@[a-zA-Z0-9_]+",
	auto_create_missing_groups: false,
};

export const MockRoleSyncSettings: TypesGen.RoleSyncSettings = {
	field: "role-test",
	mapping: {
		"idp-role-1": ["admin", "developer"],
		"idp-role-2": ["auditor"],
	},
};

export const MockOrganizationSyncSettings: TypesGen.OrganizationSyncSettings = {
	field: "organization-test",
	mapping: {
		"idp-org-1": [
			"fbd2116a-8961-4954-87ae-e4575bd29ce0",
			"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
		],
		"idp-org-2": ["fbd2116a-8961-4954-87ae-e4575bd29ce0"],
	},
	organization_assign_default: true,
};

export const MockOrganizationSyncSettings2: TypesGen.OrganizationSyncSettings =
	{
		field: "organization-test",
		mapping: {
			"idp-org-1": ["my-organization-id", "my-organization-2-id"],
			"idp-org-2": ["my-organization-id"],
		},
		organization_assign_default: true,
	};

export const MockOrganizationSyncSettingsEmpty: TypesGen.OrganizationSyncSettings =
	{
		field: "",
		mapping: {},
		organization_assign_default: true,
	};

export const MockGroup: TypesGen.Group = {
	id: "fbd2116a-8961-4954-87ae-e4575bd29ce0",
	name: "Front-End",
	display_name: "Front-End",
	avatar_url: "https://example.com",
	organization_id: MockOrganization.id,
	organization_name: MockOrganization.name,
	organization_display_name: MockOrganization.display_name,
	members: [MockUser, MockUser2],
	quota_allowance: 5,
	source: "user",
	total_member_count: 2,
};

export const MockGroup2: TypesGen.Group = {
	id: "13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
	name: "developer",
	display_name: "",
	avatar_url: "https://example.com",
	organization_id: MockOrganization.id,
	organization_name: MockOrganization.name,
	organization_display_name: MockOrganization.display_name,
	members: [MockUser, MockUser2],
	quota_allowance: 5,
	source: "user",
	total_member_count: 2,
};

const MockEveryoneGroup: TypesGen.Group = {
	// The "Everyone" group must have the same ID as a the organization it belongs
	// to.
	id: MockOrganization.id,
	name: "Everyone",
	display_name: "",
	organization_id: MockOrganization.id,
	organization_name: MockOrganization.name,
	organization_display_name: MockOrganization.display_name,
	members: [],
	avatar_url: "",
	quota_allowance: 0,
	source: "user",
	total_member_count: 0,
};

export const MockTemplateACL: TypesGen.TemplateACL = {
	group: [
		{ ...MockEveryoneGroup, role: "use" },
		{ ...MockGroup, role: "admin" },
	],
	users: [{ ...MockUser, role: "use" }],
};

export const MockTemplateACLEmpty: TypesGen.TemplateACL = {
	group: [],
	users: [],
};

export const MockTemplateExample: TypesGen.TemplateExample = {
	id: "aws-windows",
	url: "https://github.com/coder/coder/tree/main/examples/templates/aws-windows",
	name: "Develop in an ECS-hosted container",
	description: "Get started with Linux development on AWS ECS.",
	markdown:
		"\n# aws-ecs\n\nThis is a sample template for running a Coder workspace on ECS. It assumes there\nis a pre-existing ECS cluster with EC2-based compute to host the workspace.\n\n## Architecture\n\nThis workspace is built using the following AWS resources:\n\n- Task definition - the container definition, includes the image, command, volume(s)\n- ECS service - manages the task definition\n\n## code-server\n\n`code-server` is installed via the `startup_script` argument in the `coder_agent`\nresource block. The `coder_app` resource is defined to access `code-server` through\nthe dashboard UI over `localhost:13337`.\n",
	icon: "/icon/aws.svg",
	tags: ["aws", "cloud"],
};

export const MockTemplateExample2: TypesGen.TemplateExample = {
	id: "aws-linux",
	url: "https://github.com/coder/coder/tree/main/examples/templates/aws-linux",
	name: "Develop in Linux on AWS EC2",
	description: "Get started with Linux development on AWS EC2.",
	markdown:
		'\n# aws-linux\n\nTo get started, run `coder templates init`. When prompted, select this template.\nFollow the on-screen instructions to proceed.\n\n## Authentication\n\nThis template assumes that coderd is run in an environment that is authenticated\nwith AWS. For example, run `aws configure import` to import credentials on the\nsystem and user running coderd.  For other ways to authenticate [consult the\nTerraform docs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs#authentication-and-configuration).\n\n## Required permissions / policy\n\nThe following sample policy allows Coder to create EC2 instances and modify\ninstances provisioned by Coder:\n\n```json\n{\n    "Version": "2012-10-17",\n    "Statement": [\n        {\n            "Sid": "VisualEditor0",\n            "Effect": "Allow",\n            "Action": [\n                "ec2:GetDefaultCreditSpecification",\n                "ec2:DescribeIamInstanceProfileAssociations",\n                "ec2:DescribeTags",\n                "ec2:CreateTags",\n                "ec2:RunInstances",\n                "ec2:DescribeInstanceCreditSpecifications",\n                "ec2:DescribeImages",\n                "ec2:ModifyDefaultCreditSpecification",\n                "ec2:DescribeVolumes"\n            ],\n            "Resource": "*"\n        },\n        {\n            "Sid": "CoderResources",\n            "Effect": "Allow",\n            "Action": [\n                "ec2:DescribeInstances",\n                "ec2:DescribeInstanceAttribute",\n                "ec2:UnmonitorInstances",\n                "ec2:TerminateInstances",\n                "ec2:StartInstances",\n                "ec2:StopInstances",\n                "ec2:DeleteTags",\n                "ec2:MonitorInstances",\n                "ec2:CreateTags",\n                "ec2:RunInstances",\n                "ec2:ModifyInstanceAttribute",\n                "ec2:ModifyInstanceCreditSpecification"\n            ],\n            "Resource": "arn:aws:ec2:*:*:instance/*",\n            "Condition": {\n                "StringEquals": {\n                    "aws:ResourceTag/Coder_Provisioned": "true"\n                }\n            }\n        }\n    ]\n}\n```\n\n## code-server\n\n`code-server` is installed via the `startup_script` argument in the `coder_agent`\nresource block. The `coder_app` resource is defined to access `code-server` through\nthe dashboard UI over `localhost:13337`.\n',
	icon: "/icon/aws.svg",
	tags: ["aws", "cloud"],
};

export const MockPermissions: Permissions = {
	createTemplates: true,
	createUser: true,
	deleteTemplates: true,
	updateTemplates: true,
	viewAllUsers: true,
	updateUsers: true,
	viewAnyAuditLog: true,
	viewDeploymentValues: true,
	editDeploymentValues: true,
	viewUpdateCheck: true,
	viewDeploymentStats: true,
	viewExternalAuthConfig: true,
	readWorkspaceProxies: true,
	editWorkspaceProxies: true,
	createOrganization: true,
	viewAnyGroup: true,
	createGroup: true,
	viewAllLicenses: true,
	viewNotificationTemplate: true,
	viewOrganizationIDPSyncSettings: true,
};

export const MockOrganizationPermissions: OrganizationPermissions = {
	viewMembers: true,
	editMembers: true,
	createGroup: true,
	viewGroups: true,
	editGroups: true,
	editSettings: true,
	viewOrgRoles: true,
	createOrgRoles: true,
	assignOrgRoles: true,
	viewProvisioners: true,
	viewIdpSyncSettings: true,
	editIdpSyncSettings: true,
};

export const MockNoOrganizationPermissions: OrganizationPermissions = {
	viewMembers: false,
	editMembers: false,
	createGroup: false,
	viewGroups: false,
	editGroups: false,
	editSettings: false,
	viewOrgRoles: false,
	createOrgRoles: false,
	assignOrgRoles: false,
	viewProvisioners: false,
	viewIdpSyncSettings: false,
	editIdpSyncSettings: false,
};

export const MockNoPermissions: Permissions = {
	createTemplates: false,
	createUser: false,
	deleteTemplates: false,
	updateTemplates: false,
	viewAllUsers: false,
	updateUsers: false,
	viewAnyAuditLog: false,
	viewDeploymentValues: false,
	editDeploymentValues: false,
	viewUpdateCheck: false,
	viewDeploymentStats: false,
	viewExternalAuthConfig: false,
	readWorkspaceProxies: false,
	editWorkspaceProxies: false,
	createOrganization: false,
	viewAnyGroup: false,
	createGroup: false,
	viewAllLicenses: false,
	viewNotificationTemplate: false,
	viewOrganizationIDPSyncSettings: false,
};

export const MockDeploymentConfig: DeploymentConfig = {
	config: {
		enable_terraform_debug_mode: true,
	},
	options: [],
};

export const MockAppearanceConfig: TypesGen.AppearanceConfig = {
	application_name: "",
	logo_url: "",
	service_banner: {
		enabled: false,
	},
	announcement_banners: [],
	docs_url: "https://coder.com/docs/@main/",
};

export const MockWorkspaceBuildParameter1: TypesGen.WorkspaceBuildParameter = {
	name: MockTemplateVersionParameter1.name,
	value: "mock-abc",
};

export const MockWorkspaceBuildParameter2: TypesGen.WorkspaceBuildParameter = {
	name: MockTemplateVersionParameter2.name,
	value: "3",
};

export const MockWorkspaceBuildParameter3: TypesGen.WorkspaceBuildParameter = {
	name: MockTemplateVersionParameter3.name,
	value: "my-database",
};

export const MockWorkspaceBuildParameter4: TypesGen.WorkspaceBuildParameter = {
	name: MockTemplateVersionParameter4.name,
	value: "immutable-value",
};

export const MockWorkspaceBuildParameter5: TypesGen.WorkspaceBuildParameter = {
	name: MockTemplateVersionParameter5.name,
	value: "5",
};

export const MockTemplateVersionExternalAuthGithub: TypesGen.TemplateVersionExternalAuth =
	{
		id: "github",
		type: "github",
		authenticate_url: "https://example.com/external-auth/github",
		authenticated: false,
		display_icon: "/icon/github.svg",
		display_name: "GitHub",
	};

export const MockTemplateVersionExternalAuthGithubAuthenticated: TypesGen.TemplateVersionExternalAuth =
	{
		id: "github",
		type: "github",
		authenticate_url: "https://example.com/external-auth/github",
		authenticated: true,
		display_icon: "/icon/github.svg",
		display_name: "GitHub",
	};

export const MockDeploymentStats: TypesGen.DeploymentStats = {
	aggregated_from: "2023-03-06T19:08:55.211625Z",
	collected_at: "2023-03-06T19:12:55.211625Z",
	next_update_at: "2023-03-06T19:20:55.211625Z",
	session_count: {
		vscode: 128,
		jetbrains: 5,
		ssh: 32,
		reconnecting_pty: 15,
	},
	workspaces: {
		building: 15,
		failed: 12,
		pending: 5,
		running: 32,
		stopped: 16,
		connection_latency_ms: {
			P50: 32.56,
			P95: 15.23,
		},
		rx_bytes: 15613513253,
		tx_bytes: 36113513253,
	},
};

export const MockDeploymentSSH: TypesGen.SSHConfigResponse = {
	hostname_prefix: " coder.",
	ssh_config_options: {},
};

export const MockWorkspaceAgentLogs: TypesGen.WorkspaceAgentLog[] = [
	{
		id: 166663,
		created_at: "2023-05-04T11:30:41.402072Z",
		output: "+ curl -fsSL https://code-server.dev/install.sh",
		level: "info",
		source_id: MockWorkspaceAgentLogSource.id,
	},
	{
		id: 166664,
		created_at: "2023-05-04T11:30:41.40228Z",
		output:
			"+ sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.8.3",
		level: "info",
		source_id: MockWorkspaceAgentLogSource.id,
	},
	{
		id: 166665,
		created_at: "2023-05-04T11:30:42.590731Z",
		output: "Ubuntu 22.04.2 LTS",
		level: "info",
		source_id: MockWorkspaceAgentLogSource.id,
	},
	{
		id: 166666,
		created_at: "2023-05-04T11:30:42.593686Z",
		output: "Installing v4.8.3 of the amd64 release from GitHub.",
		level: "info",
		source_id: MockWorkspaceAgentLogSource.id,
	},
];

export const MockLicenseResponse: GetLicensesResponse[] = [
	{
		id: 1,
		uploaded_at: "1660104000",
		expires_at: "3420244800", // expires on 5/20/2078
		uuid: "1",
		claims: {
			trial: false,
			all_features: true,
			feature_set: "enterprise",
			version: 1,
			features: {},
			license_expires: 3420244800,
		},
	},
	{
		id: 1,
		uploaded_at: "1660104000",
		expires_at: "3420244800", // expires on 5/20/2078
		uuid: "1",
		claims: {
			trial: false,
			all_features: true,
			feature_set: "PREMIUM",
			version: 1,
			features: {},
			license_expires: 3420244800,
		},
	},
	{
		id: 1,
		uploaded_at: "1660104000",
		expires_at: "3420244800", // expires on 5/20/2078
		uuid: "1",
		claims: {
			trial: false,
			all_features: true,
			version: 1,
			features: {},
			license_expires: 3420244800,
		},
	},
	{
		id: 1,
		uploaded_at: "1660104000",
		expires_at: "1660104000", // expired on 8/10/2022
		uuid: "1",
		claims: {
			trial: false,
			all_features: true,
			version: 1,
			features: {},
			license_expires: 1660104000,
		},
	},
	{
		id: 1,
		uploaded_at: "1682346425",
		expires_at: "1682346425", // expired on 4/24/2023
		uuid: "1",
		claims: {
			trial: false,
			all_features: true,
			version: 1,
			features: {},
			license_expires: 1682346425,
		},
	},
];

export const MockHealth: TypesGen.HealthcheckReport = {
	time: "2023-08-01T16:51:03.29792825Z",
	healthy: true,
	severity: "ok",
	derp: {
		healthy: true,
		severity: "ok",
		warnings: [],
		dismissed: false,
		regions: {
			"999": {
				healthy: true,
				severity: "ok",
				warnings: [],
				region: {
					EmbeddedRelay: true,
					RegionID: 999,
					RegionCode: "coder",
					RegionName: "Council Bluffs, Iowa",
					Nodes: [
						{
							Name: "999stun0",
							RegionID: 999,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						{
							Name: "999b",
							RegionID: 999,
							HostName: "dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
					],
				},
				node_reports: [
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "999stun0",
							RegionID: 999,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: false,
						round_trip_ping: "0",
						round_trip_ping_ms: 0,
						uses_websocket: false,
						client_logs: [],
						client_errs: [],
						stun: {
							Enabled: true,
							CanSTUN: true,
							Error: null,
						},
					},
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "999b",
							RegionID: 999,
							HostName: "dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: true,
						round_trip_ping: "7674330",
						round_trip_ping_ms: 7674330,
						uses_websocket: false,
						client_logs: [
							[
								"derphttp.Client.Connect: connecting to https://dev.coder.com/derp",
							],
							[
								"derphttp.Client.Connect: connecting to https://dev.coder.com/derp",
							],
						],
						client_errs: [
							["recv derp message: derphttp.Client closed"],
							[
								"connect to derp: derphttp.Client.Connect connect to <https://sao-paulo.fly.dev.coder.com/derp>: context deadline exceeded: read tcp 10.44.1.150:59546-&gt;149.248.214.149:443: use of closed network connection",
								"connect to derp: derphttp.Client closed",
								"connect to derp: derphttp.Client closed",
								"connect to derp: derphttp.Client closed",
								"connect to derp: derphttp.Client closed",
								"couldn't connect after 5 tries, last error: couldn't connect after 5 tries, last error: derphttp.Client closed",
							],
						],
						stun: {
							Enabled: false,
							CanSTUN: false,
							Error: null,
						},
					},
				],
			},
			"10007": {
				healthy: true,
				severity: "ok",
				warnings: [],
				region: {
					EmbeddedRelay: false,
					RegionID: 10007,
					RegionCode: "coder_sydney",
					RegionName: "sydney",
					Nodes: [
						{
							Name: "10007stun0",
							RegionID: 10007,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						{
							Name: "10007a",
							RegionID: 10007,
							HostName: "sydney.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
					],
				},
				node_reports: [
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10007stun0",
							RegionID: 10007,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: false,
						round_trip_ping: "0",
						round_trip_ping_ms: 0,
						uses_websocket: false,
						client_logs: [],
						client_errs: [],
						stun: {
							Enabled: true,
							CanSTUN: true,
							Error: null,
						},
					},
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10007a",
							RegionID: 10007,
							HostName: "sydney.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: true,
						round_trip_ping: "170527034",
						round_trip_ping_ms: 170527034,
						uses_websocket: false,
						client_logs: [
							[
								"derphttp.Client.Connect: connecting to https://sydney.dev.coder.com/derp",
							],
							[
								"derphttp.Client.Connect: connecting to https://sydney.dev.coder.com/derp",
							],
						],
						client_errs: [[], []],
						stun: {
							Enabled: false,
							CanSTUN: false,
							Error: null,
						},
					},
				],
			},
			"10008": {
				healthy: true,
				severity: "ok",
				warnings: [],
				region: {
					EmbeddedRelay: false,
					RegionID: 10008,
					RegionCode: "coder_europe-frankfurt",
					RegionName: "europe-frankfurt",
					Nodes: [
						{
							Name: "10008stun0",
							RegionID: 10008,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						{
							Name: "10008a",
							RegionID: 10008,
							HostName: "europe.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
					],
				},
				node_reports: [
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10008stun0",
							RegionID: 10008,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: false,
						round_trip_ping: "0",
						round_trip_ping_ms: 0,
						uses_websocket: false,
						client_logs: [],
						client_errs: [],
						stun: {
							Enabled: true,
							CanSTUN: true,
							Error: null,
						},
					},
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10008a",
							RegionID: 10008,
							HostName: "europe.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: true,
						round_trip_ping: "111329690",
						round_trip_ping_ms: 111329690,
						uses_websocket: false,
						client_logs: [
							[
								"derphttp.Client.Connect: connecting to https://europe.dev.coder.com/derp",
							],
							[
								"derphttp.Client.Connect: connecting to https://europe.dev.coder.com/derp",
							],
						],
						client_errs: [[], []],
						stun: {
							Enabled: false,
							CanSTUN: false,
							Error: null,
						},
					},
				],
			},
			"10009": {
				healthy: true,
				severity: "ok",
				warnings: [],
				region: {
					EmbeddedRelay: false,
					RegionID: 10009,
					RegionCode: "coder_brazil-saopaulo",
					RegionName: "brazil-saopaulo",
					Nodes: [
						{
							Name: "10009stun0",
							RegionID: 10009,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						{
							Name: "10009a",
							RegionID: 10009,
							HostName: "brazil.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
					],
				},
				node_reports: [
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10009stun0",
							RegionID: 10009,
							HostName: "stun.l.google.com",
							STUNPort: 19302,
							STUNOnly: true,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: false,
						round_trip_ping: "0",
						round_trip_ping_ms: 0,
						uses_websocket: false,
						client_logs: [],
						client_errs: [],
						stun: {
							Enabled: true,
							CanSTUN: true,
							Error: null,
						},
					},
					{
						healthy: true,
						severity: "ok",
						warnings: [],
						node: {
							Name: "10009a",
							RegionID: 10009,
							HostName: "brazil.dev.coder.com",
							STUNPort: -1,
							DERPPort: 443,
						},
						node_info: {
							TokenBucketBytesPerSecond: 0,
							TokenBucketBytesBurst: 0,
						},
						can_exchange_messages: true,
						round_trip_ping: "138185506",
						round_trip_ping_ms: 138185506,
						uses_websocket: false,
						client_logs: [
							[
								"derphttp.Client.Connect: connecting to https://brazil.dev.coder.com/derp",
							],
							[
								"derphttp.Client.Connect: connecting to https://brazil.dev.coder.com/derp",
							],
						],
						client_errs: [[], []],
						stun: {
							Enabled: false,
							CanSTUN: false,
							Error: null,
						},
					},
				],
			},
		},
		netcheck: {
			UDP: true,
			IPv6: false,
			IPv4: true,
			IPv6CanSend: false,
			IPv4CanSend: true,
			OSHasIPv6: true,
			ICMPv4: false,
			MappingVariesByDestIP: false,
			HairPinning: null,
			UPnP: false,
			PMP: false,
			PCP: false,
			PreferredDERP: 999,
			RegionLatency: {
				"999": 1638180,
				"10007": 174853022,
				"10008": 112142029,
				"10009": 138855606,
			},
			RegionV4Latency: {
				"999": 1638180,
				"10007": 174853022,
				"10008": 112142029,
				"10009": 138855606,
			},
			RegionV6Latency: {},
			GlobalV4: "34.71.26.24:55368",
			GlobalV6: "",
			CaptivePortal: null,
		},
		netcheck_logs: [
			"netcheck: netcheck.runProbe: got STUN response for 10007stun0 from 34.71.26.24:55368 (9b07930007da49dd7df79bc7) in 1.791799ms",
			"netcheck: netcheck.runProbe: got STUN response for 999stun0 from 34.71.26.24:55368 (7397fec097f1d5b01364566b) in 1.791529ms",
			"netcheck: netcheck.runProbe: got STUN response for 10008stun0 from 34.71.26.24:55368 (1fdaaa016ca386485f097f68) in 2.192899ms",
			"netcheck: netcheck.runProbe: got STUN response for 10009stun0 from 34.71.26.24:55368 (2596fe60895fbd9542823a76) in 2.146459ms",
			"netcheck: netcheck.runProbe: got STUN response for 10007stun0 from 34.71.26.24:55368 (19ec320f3b76e8b027b06d3e) in 2.139619ms",
			"netcheck: netcheck.runProbe: got STUN response for 999stun0 from 34.71.26.24:55368 (a17973bc57c35e606c0f46f5) in 2.131089ms",
			"netcheck: netcheck.runProbe: got STUN response for 10008stun0 from 34.71.26.24:55368 (c958e15209d139a6e410f13a) in 2.127549ms",
			"netcheck: netcheck.runProbe: got STUN response for 10009stun0 from 34.71.26.24:55368 (284a1b64dff22f40a3514524) in 2.107549ms",
			"netcheck: [v1] measureAllICMPLatency: listen ip4:icmp 0.0.0.0: socket: operation not permitted",
			"netcheck: [v1] report: udp=true v6=false v6os=true mapvarydest=false hair= portmap= v4a=34.71.26.24:55368 derp=999 derpdist=999v4:2ms,10007v4:175ms,10008v4:112ms,10009v4:139ms",
		],
	},
	access_url: {
		healthy: true,
		severity: "ok",
		warnings: [],
		dismissed: false,
		access_url: "https://dev.coder.com",
		reachable: true,
		status_code: 200,
		healthz_response: "OK",
	},
	websocket: {
		healthy: true,
		severity: "ok",
		warnings: [],
		dismissed: false,
		body: "",
		code: 101,
	},
	database: {
		healthy: true,
		severity: "ok",
		warnings: [],
		dismissed: false,
		reachable: true,
		latency: "92570",
		latency_ms: 92570,
		threshold_ms: 92570,
	},
	workspace_proxy: {
		healthy: true,
		severity: "warning",
		warnings: [
			{
				code: "EWP04",
				message:
					'unhealthy: request to proxy failed: Get "http://127.0.0.1:3001/healthz-report": dial tcp 127.0.0.1:3001: connect: connection refused',
			},
		],
		dismissed: false,
		error: undefined,
		workspace_proxies: {
			regions: [
				{
					id: "1a3e5eb8-d785-4f7d-9188-2eeab140cd06",
					name: "primary",
					display_name: "Council Bluffs, Iowa",
					icon_url: "/emojis/1f3e1.png",
					healthy: true,
					path_app_url: "https://dev.coder.com",
					wildcard_hostname: "*--apps.dev.coder.com",
					derp_enabled: false,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.829032482Z",
					},
					created_at: "0001-01-01T00:00:00Z",
					updated_at: "0001-01-01T00:00:00Z",
					deleted: false,
					version: "",
				},
				{
					id: "2876ab4d-bcee-4643-944f-d86323642840",
					name: "sydney",
					display_name: "Sydney GCP",
					icon_url: "/emojis/1f1e6-1f1fa.png",
					healthy: true,
					path_app_url: "https://sydney.dev.coder.com",
					wildcard_hostname: "*--apps.sydney.dev.coder.com",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-05-01T19:15:56.606593Z",
					updated_at: "2023-12-05T14:13:36.647535Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
				{
					id: "9d786ce0-55b1-4ace-8acc-a4672ff8d41f",
					name: "europe-frankfurt",
					display_name: "Europe GCP (Frankfurt)",
					icon_url: "/emojis/1f1e9-1f1ea.png",
					healthy: true,
					path_app_url: "https://europe.dev.coder.com",
					wildcard_hostname: "*--apps.europe.dev.coder.com",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-05-01T20:34:11.114005Z",
					updated_at: "2023-12-05T14:13:45.941716Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
				{
					id: "2e209786-73b1-4838-ba78-e01c9334450a",
					name: "brazil-saopaulo",
					display_name: "Brazil GCP (Sao Paulo)",
					icon_url: "/emojis/1f1e7-1f1f7.png",
					healthy: true,
					path_app_url: "https://brazil.dev.coder.com",
					wildcard_hostname: "*--apps.brazil.dev.coder.com",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-05-01T20:41:02.76448Z",
					updated_at: "2023-12-05T14:13:41.968568Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
				{
					id: "c272e80c-0cce-49d6-9782-1b5cf90398e8",
					name: "unregistered",
					display_name: "UnregisteredProxy",
					icon_url: "/emojis/274c.png",
					healthy: false,
					path_app_url: "",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "unregistered",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-07-10T14:51:11.539222Z",
					updated_at: "2023-07-10T14:51:11.539223Z",
					deleted: false,
					version: "",
				},
				{
					id: "a3efbff1-587b-4677-80a4-dc4f892fed3e",
					name: "unhealthy",
					display_name: "Unhealthy",
					icon_url: "/emojis/1f92e.png",
					healthy: false,
					path_app_url: "http://127.0.0.1:3001",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "unreachable",
						report: {
							errors: [
								'request to proxy failed: Get "http://127.0.0.1:3001/healthz-report": dial tcp 127.0.0.1:3001: connect: connection refused',
							],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-07-10T14:51:48.407017Z",
					updated_at: "2023-07-10T14:51:57.993682Z",
					deleted: false,
					version: "",
				},
				{
					id: "b6cefb69-cb6f-46e2-9c9c-39c089fb7e42",
					name: "paris-coder",
					display_name: "Europe (Paris)",
					icon_url: "/emojis/1f1eb-1f1f7.png",
					healthy: true,
					path_app_url: "https://paris-coder.fly.dev",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-12-01T09:21:15.996267Z",
					updated_at: "2023-12-05T14:13:59.663174Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
				{
					id: "72649dc9-03c7-46a8-bc95-96775e93ddc1",
					name: "sydney-coder",
					display_name: "Australia (Sydney)",
					icon_url: "/emojis/1f1e6-1f1fa.png",
					healthy: true,
					path_app_url: "https://sydney-coder.fly.dev",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-12-01T09:23:44.505529Z",
					updated_at: "2023-12-05T14:13:55.769058Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
				{
					id: "1f78398f-e5ae-4c38-aa89-30222181d443",
					name: "sao-paulo-coder",
					display_name: "Brazil (Sau Paulo)",
					icon_url: "/emojis/1f1e7-1f1f7.png",
					healthy: true,
					path_app_url: "https://sao-paulo-coder.fly.dev",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "ok",
						report: {
							errors: [],
							warnings: [],
						},
						checked_at: "2023-12-05T14:14:05.250322277Z",
					},
					created_at: "2023-12-01T09:36:00.231252Z",
					updated_at: "2023-12-05T14:13:47.015031Z",
					deleted: false,
					version: MockBuildInfo.version,
				},
			],
		},
	},
	provisioner_daemons: {
		severity: "ok",
		warnings: [
			{
				message: "Something is wrong!",
				code: "EUNKNOWN",
			},
			{
				message: "This is also bad.",
				code: "EPD01",
			},
		],
		dismissed: false,
		items: [
			{
				provisioner_daemon: {
					id: "e455b582-ac04-4323-9ad6-ab71301fa006",
					organization_id: MockOrganization.id,
					key_id: MockProvisionerKey.id,
					created_at: "2024-01-04T15:53:03.21563Z",
					last_seen_at: "2024-01-04T16:05:03.967551Z",
					name: "ok",
					version: MockBuildInfo.version,
					api_version: MockBuildInfo.provisioner_api_version,
					provisioners: ["echo", "terraform"],
					tags: {
						owner: "",
						scope: "organization",
						tag_value: "value",
						tag_true: "true",
						tag_1: "1",
						tag_yes: "yes",
					},
					key_name: MockProvisionerKey.name,
					current_job: null,
					previous_job: null,
					status: "idle",
				},
				warnings: [],
			},
			{
				provisioner_daemon: {
					id: "00000000-0000-0000-000000000000",
					organization_id: MockOrganization.id,
					key_id: MockProvisionerKey.id,
					created_at: "2024-01-04T15:53:03.21563Z",
					last_seen_at: "2024-01-04T16:05:03.967551Z",
					name: "user-scoped",
					version: MockBuildInfo.version,
					api_version: MockBuildInfo.provisioner_api_version,
					provisioners: ["echo", "terraform"],
					tags: {
						owner: "12345678-1234-1234-1234-12345678abcd",
						scope: "user",
						tag_VALUE: "VALUE",
						tag_TRUE: "TRUE",
						tag_1: "1",
						tag_YES: "YES",
					},
					key_name: MockProvisionerKey.name,
					current_job: null,
					previous_job: null,
					status: "idle",
				},
				warnings: [],
			},
			{
				provisioner_daemon: {
					id: "e455b582-ac04-4323-9ad6-ab71301fa006",
					organization_id: MockOrganization.id,
					key_id: MockProvisionerKey.id,
					created_at: "2024-01-04T15:53:03.21563Z",
					last_seen_at: "2024-01-04T16:05:03.967551Z",
					name: "unhappy",
					version: "v0.0.1",
					api_version: "0.1",
					provisioners: ["echo", "terraform"],
					tags: {
						owner: "",
						scope: "organization",
						tag_string: "value",
						tag_false: "false",
						tag_0: "0",
						tag_no: "no",
					},
					key_name: MockProvisionerKey.name,
					current_job: null,
					previous_job: null,
					status: "idle",
				},
				warnings: [
					{
						message: "Something specific is wrong with this daemon.",
						code: "EUNKNOWN",
					},
					{
						message: "And now for something completely different.",
						code: "EUNKNOWN",
					},
				],
			},
		],
	},
	coder_version: MockBuildInfo.version,
};

export const MockListeningPortsResponse: TypesGen.WorkspaceAgentListeningPortsResponse =
	{
		ports: [
			{ process_name: "webb", network: "", port: 30000 },
			{ process_name: "gogo", network: "", port: 8080 },
			{ process_name: "", network: "", port: 8081 },
		],
	};

export const MockSharedPortsResponse: TypesGen.WorkspaceAgentPortShares = {
	shares: [
		{
			workspace_id: MockWorkspace.id,
			agent_name: "a-workspace-agent",
			port: 4000,
			share_level: "authenticated",
			protocol: "http",
		},
		{
			workspace_id: MockWorkspace.id,
			agent_name: "a-workspace-agent",
			port: 65535,
			share_level: "authenticated",
			protocol: "https",
		},
		{
			workspace_id: MockWorkspace.id,
			agent_name: "a-workspace-agent",
			port: 8081,
			share_level: "public",
			protocol: "http",
		},
	],
};

export const DeploymentHealthUnhealthy: TypesGen.HealthcheckReport = {
	healthy: false,
	severity: "ok",
	time: "2023-10-12T23:15:00.000000000Z",
	coder_version: "v2.3.0-devel+8cca4915a",
	access_url: {
		healthy: true,
		severity: "ok",
		warnings: [],
		dismissed: false,
		access_url: "",
		healthz_response: "",
		reachable: true,
		status_code: 0,
	},
	database: {
		healthy: false,
		severity: "ok",
		warnings: [],
		dismissed: false,
		latency: "",
		latency_ms: 0,
		reachable: true,
		threshold_ms: 92570,
	},
	derp: {
		healthy: false,
		severity: "ok",
		warnings: [],
		dismissed: false,
		regions: [],
		netcheck_logs: [],
	},
	websocket: {
		healthy: false,
		severity: "ok",
		warnings: [],
		dismissed: false,
		body: "",
		code: 0,
	},
	workspace_proxy: {
		healthy: false,
		error: "some error",
		severity: "error",
		warnings: [],
		dismissed: false,
		workspace_proxies: {
			regions: [
				{
					id: "df7e4b2b-2d40-47e5-a021-e5d08b219c77",
					name: "unhealthy",
					display_name: "unhealthy",
					icon_url: "/emojis/1f5fa.png",
					healthy: false,
					path_app_url: "http://127.0.0.1:3001",
					wildcard_hostname: "",
					derp_enabled: true,
					derp_only: false,
					status: {
						status: "unreachable",
						report: {
							errors: ["some error"],
							warnings: [],
						},
						checked_at: "2023-11-24T12:14:05.743303497Z",
					},
					created_at: "2023-11-23T15:37:25.513213Z",
					updated_at: "2023-11-23T18:09:19.734747Z",
					deleted: false,
					version: "v2.5.0-devel+89bae7eff",
				},
			],
		},
	},
	provisioner_daemons: {
		severity: "error",
		error: "something went wrong",
		warnings: [
			{
				message: "this is a message",
				code: "EUNKNOWN",
			},
		],
		dismissed: false,
		items: [
			{
				provisioner_daemon: {
					id: "e455b582-ac04-4323-9ad6-ab71301fa006",
					organization_id: MockOrganization.id,
					key_id: MockProvisionerKey.id,
					created_at: "2024-01-04T15:53:03.21563Z",
					last_seen_at: "2024-01-04T16:05:03.967551Z",
					name: "vvuurrkk-2",
					version: "v2.6.0-devel+965ad5e96",
					api_version: "1.0",
					provisioners: ["echo", "terraform"],
					tags: {
						owner: "",
						scope: "organization",
					},
					key_name: MockProvisionerKey.name,
					current_job: null,
					previous_job: null,
					status: "idle",
				},
				warnings: [
					{
						message: "this is a specific message for this thing",
						code: "EUNKNOWN",
					},
				],
			},
		],
	},
};

export const MockHealthSettings: TypesGen.HealthSettings = {
	dismissed_healthchecks: [],
};

export const MockGithubExternalProvider: TypesGen.ExternalAuthLinkProvider = {
	id: "github",
	type: "github",
	device: false,
	display_icon: "/icon/github.svg",
	display_name: "GitHub",
	allow_refresh: true,
	allow_validate: true,
};

export const MockGithubAuthLink: TypesGen.ExternalAuthLink = {
	provider_id: "github",
	created_at: "",
	updated_at: "",
	has_refresh_token: true,
	expires: "",
	authenticated: true,
	validate_error: "",
};

export const MockOAuth2ProviderApps: TypesGen.OAuth2ProviderApp[] = [
	{
		id: "1",
		name: "foo",
		callback_url: "http://localhost:3001",
		icon: "/icon/github.svg",
		endpoints: {
			authorization: "http://localhost:3001/oauth2/authorize",
			token: "http://localhost:3001/oauth2/token",
			device_authorization: "",
		},
	},
];

export const MockOAuth2ProviderAppSecrets: TypesGen.OAuth2ProviderAppSecret[] =
	[
		{
			id: "1",
			client_secret_truncated: "foo",
			last_used_at: null,
		},
		{
			id: "1",
			last_used_at: "2022-12-16T20:10:45.637452Z",
			client_secret_truncated: "foo",
		},
	];

export const MockNotificationPreferences: TypesGen.NotificationPreference[] = [
	{
		id: "f44d9314-ad03-4bc8-95d0-5cad491da6b6",
		disabled: false,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "381df2a9-c0c0-4749-420f-80a9280c66f9",
		disabled: true,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "f517da0b-cdc9-410f-ab89-a86107c420ed",
		disabled: false,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "c34a0c09-0704-4cac-bd1c-0c0146811c2b",
		disabled: false,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "0ea69165-ec14-4314-91f1-69566ac3c5a0",
		disabled: false,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "51ce2fdf-c9ca-4be1-8d70-628674f9bc42",
		disabled: false,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
	{
		id: "4e19c0ac-94e1-4532-9515-d1801aa283b2",
		disabled: true,
		updated_at: "2024-08-06T11:58:37.755053Z",
	},
];

export const MockNotificationTemplates: TypesGen.NotificationTemplate[] = [
	{
		id: "381df2a9-c0c0-4749-420f-80a9280c66f9",
		name: "Workspace Autobuild Failed",
		title_template: 'Workspace "{{.Labels.name}}" autobuild failed',
		body_template:
			'Hi {{.UserName}}\nAutomatic build of your workspace **{{.Labels.name}}** failed.\nThe specified reason was "**{{.Labels.reason}}**".',
		actions:
			'[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
		group: "Workspace Events",
		method: "webhook",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "f517da0b-cdc9-410f-ab89-a86107c420ed",
		name: "Workspace Deleted",
		title_template: 'Workspace "{{.Labels.name}}" deleted',
		body_template:
			'Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** was deleted.\nThe specified reason was "**{{.Labels.reason}}{{ if .Labels.initiator }} ({{ .Labels.initiator }}){{end}}**".',
		actions:
			'[{"url": "{{ base_url }}/workspaces", "label": "View workspaces"}, {"url": "{{ base_url }}/templates", "label": "View templates"}]',
		group: "Workspace Events",
		method: "smtp",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "f44d9314-ad03-4bc8-95d0-5cad491da6b6",
		name: "User account deleted",
		title_template: 'User account "{{.Labels.deleted_account_name}}" deleted',
		body_template:
			"Hi {{.UserName}},\n\nUser account **{{.Labels.deleted_account_name}}** has been deleted.",
		actions:
			'[{"url": "{{ base_url }}/deployment/users?filter=status%3Aactive", "label": "View accounts"}]',
		group: "User Events",
		method: "",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "4e19c0ac-94e1-4532-9515-d1801aa283b2",
		name: "User account created",
		title_template: 'User account "{{.Labels.created_account_name}}" created',
		body_template:
			"Hi {{.UserName}},\n\nNew user account **{{.Labels.created_account_name}}** has been created.",
		actions:
			'[{"url": "{{ base_url }}/deployment/users?filter=status%3Aactive", "label": "View accounts"}]',
		group: "User Events",
		method: "",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "0ea69165-ec14-4314-91f1-69566ac3c5a0",
		name: "Workspace Marked as Dormant",
		title_template: 'Workspace "{{.Labels.name}}" marked as dormant',
		body_template:
			"Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.\nDormant workspaces are [automatically deleted](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) after {{.Labels.timeTilDormant}} of inactivity.\nTo prevent deletion, use your workspace with the link below.",
		actions:
			'[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
		group: "Workspace Events",
		method: "smtp",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "c34a0c09-0704-4cac-bd1c-0c0146811c2b",
		name: "Workspace updated automatically",
		title_template: 'Workspace "{{.Labels.name}}" updated automatically',
		body_template:
			"Hi {{.UserName}}\nYour workspace **{{.Labels.name}}** has been updated automatically to the latest template version ({{.Labels.template_version_name}}).",
		actions:
			'[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
		group: "Workspace Events",
		method: "smtp",
		kind: "system",
		enabled_by_default: true,
	},
	{
		id: "51ce2fdf-c9ca-4be1-8d70-628674f9bc42",
		name: "Workspace Marked for Deletion",
		title_template: 'Workspace "{{.Labels.name}}" marked for deletion',
		body_template:
			"Hi {{.UserName}}\n\nYour workspace **{{.Labels.name}}** has been marked for **deletion** after {{.Labels.timeTilDormant}} of [dormancy](https://coder.com/docs/templates/schedule#dormancy-auto-deletion-enterprise) because of {{.Labels.reason}}.\nTo prevent deletion, use your workspace with the link below.",
		actions:
			'[{"url": "{{ base_url }}/@{{.UserUsername}}/{{.Labels.name}}", "label": "View workspace"}]',
		group: "Workspace Events",
		method: "webhook",
		kind: "system",
		enabled_by_default: true,
	},
];

export const MockNotificationMethodsResponse: TypesGen.NotificationMethodsResponse =
	{ available: ["smtp", "webhook"], default: "smtp" };
