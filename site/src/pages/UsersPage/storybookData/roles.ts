import type { AssignableRoles, RBACAction, Role } from "api/typesGenerated";

// The following values were retrieved from the Coder API.
export const MockRoles: (AssignableRoles | Role)[] = [
	{
		name: "owner",
		display_name: "Owner",
		site_permissions: [
			{
				negate: false,
				resource_type: "api_key",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "assign_org_role",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "assign_role",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "audit_log",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "debug_info",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "deployment_config",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "deployment_stats",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "file",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "group",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "group_member",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "license",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "notification_preference",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "notification_template",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "oauth2_app",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "oauth2_app_code_token",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "oauth2_app_secret",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "organization",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "provisioner_daemon",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "replicas",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "system",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "tailnet_coordinator",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "template",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "user",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "workspace_proxy",
				action: "*" as RBACAction,
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "update",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "start",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "stop",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "ssh",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "application_connect",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "create",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "read",
			},
			{
				negate: false,
				resource_type: "workspace_dormant",
				action: "read",
			},
			{
				negate: false,
				resource_type: "workspace_dormant",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "workspace_dormant",
				action: "create",
			},
			{
				negate: false,
				resource_type: "workspace_dormant",
				action: "update",
			},
			{
				negate: false,
				resource_type: "workspace_dormant",
				action: "stop",
			},
		],
		organization_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
	{
		name: "template-admin",
		display_name: "Template Admin",
		site_permissions: [
			{
				negate: false,
				resource_type: "file",
				action: "read",
			},
			{
				negate: false,
				resource_type: "file",
				action: "create",
			},
			{
				negate: false,
				resource_type: "group",
				action: "read",
			},
			{
				negate: false,
				resource_type: "group_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "organization",
				action: "read",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "provisioner_daemon",
				action: "update",
			},
			{
				negate: false,
				resource_type: "provisioner_daemon",
				action: "read",
			},
			{
				negate: false,
				resource_type: "provisioner_daemon",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "provisioner_daemon",
				action: "create",
			},
			{
				negate: false,
				resource_type: "template",
				action: "create",
			},
			{
				negate: false,
				resource_type: "template",
				action: "view_insights",
			},
			{
				negate: false,
				resource_type: "template",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "template",
				action: "update",
			},
			{
				negate: false,
				resource_type: "template",
				action: "read",
			},
			{
				negate: false,
				resource_type: "user",
				action: "read",
			},
			{
				negate: false,
				resource_type: "workspace",
				action: "read",
			},
		],
		organization_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
	{
		name: "user-admin",
		display_name: "User Admin",
		site_permissions: [
			{
				negate: false,
				resource_type: "assign_org_role",
				action: "assign",
			},
			{
				negate: false,
				resource_type: "assign_org_role",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "assign_org_role",
				action: "read",
			},
			{
				negate: false,
				resource_type: "assign_role",
				action: "assign",
			},
			{
				negate: false,
				resource_type: "assign_role",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "assign_role",
				action: "read",
			},
			{
				negate: false,
				resource_type: "group",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "group",
				action: "update",
			},
			{
				negate: false,
				resource_type: "group",
				action: "read",
			},
			{
				negate: false,
				resource_type: "group",
				action: "create",
			},
			{
				negate: false,
				resource_type: "group_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "create",
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
				resource_type: "user",
				action: "read_personal",
			},
			{
				negate: false,
				resource_type: "user",
				action: "update_personal",
			},
			{
				negate: false,
				resource_type: "user",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "user",
				action: "update",
			},
			{
				negate: false,
				resource_type: "user",
				action: "read",
			},
			{
				negate: false,
				resource_type: "user",
				action: "create",
			},
		],
		organization_permissions: [],
		user_permissions: [],
		assignable: true,
		built_in: true,
	},
	{
		name: "auditor",
		display_name: "Auditor",
		site_permissions: [
			{
				negate: false,
				resource_type: "audit_log",
				action: "read",
			},
			{
				negate: false,
				resource_type: "deployment_config",
				action: "read",
			},
			{
				negate: false,
				resource_type: "deployment_stats",
				action: "read",
			},
			{
				negate: false,
				resource_type: "group",
				action: "read",
			},
			{
				negate: false,
				resource_type: "group_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "template",
				action: "read",
			},
			{
				negate: false,
				resource_type: "template",
				action: "view_insights",
			},
			{
				negate: false,
				resource_type: "user",
				action: "read",
			},
		],
		organization_permissions: [],
		user_permissions: [],
		built_in: true,
	},
];
