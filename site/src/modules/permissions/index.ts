import type { AuthorizationCheck } from "api/typesGenerated";

export type Permissions = {
	[k in PermissionName]: boolean;
};

export type PermissionName = keyof typeof permissionChecks;

/**
 * Site-wide permission checks
 */
export const permissionChecks = {
	viewAllUsers: {
		object: {
			resource_type: "user",
		},
		action: "read",
	},
	updateUsers: {
		object: {
			resource_type: "user",
		},
		action: "update",
	},
	createUser: {
		object: {
			resource_type: "user",
		},
		action: "create",
	},
	createTemplates: {
		object: {
			resource_type: "template",
			any_org: true,
		},
		action: "create",
	},
	updateTemplates: {
		object: {
			resource_type: "template",
		},
		action: "update",
	},
	deleteTemplates: {
		object: {
			resource_type: "template",
		},
		action: "delete",
	},
	viewDeploymentConfig: {
		object: {
			resource_type: "deployment_config",
		},
		action: "read",
	},
	editDeploymentConfig: {
		object: {
			resource_type: "deployment_config",
		},
		action: "update",
	},
	viewDeploymentStats: {
		object: {
			resource_type: "deployment_stats",
		},
		action: "read",
	},
	readWorkspaceProxies: {
		object: {
			resource_type: "workspace_proxy",
		},
		action: "read",
	},
	editWorkspaceProxies: {
		object: {
			resource_type: "workspace_proxy",
		},
		action: "create",
	},
	createOrganization: {
		object: {
			resource_type: "organization",
		},
		action: "create",
	},
	viewAnyGroup: {
		object: {
			resource_type: "group",
		},
		action: "read",
	},
	createGroup: {
		object: {
			resource_type: "group",
		},
		action: "create",
	},
	viewAllLicenses: {
		object: {
			resource_type: "license",
		},
		action: "read",
	},
	viewNotificationTemplate: {
		object: {
			resource_type: "notification_template",
		},
		action: "read",
	},
	viewOrganizationIDPSyncSettings: {
		object: {
			resource_type: "idpsync_settings",
		},
		action: "read",
	},

	viewAnyMembers: {
		object: {
			resource_type: "organization_member",
			any_org: true,
		},
		action: "read",
	},
	editAnyGroups: {
		object: {
			resource_type: "group",
			any_org: true,
		},
		action: "update",
	},
	assignAnyRoles: {
		object: {
			resource_type: "assign_org_role",
			any_org: true,
		},
		action: "assign",
	},
	viewAnyIdpSyncSettings: {
		object: {
			resource_type: "idpsync_settings",
			any_org: true,
		},
		action: "read",
	},
	editAnySettings: {
		object: {
			resource_type: "organization",
			any_org: true,
		},
		action: "update",
	},
	viewAnyAuditLog: {
		object: {
			resource_type: "audit_log",
			any_org: true,
		},
		action: "read",
	},
	viewDebugInfo: {
		object: {
			resource_type: "debug_info",
		},
		action: "read",
	},
} as const satisfies Record<string, AuthorizationCheck>;

export const canViewDeploymentSettings = (
	permissions: Permissions | undefined,
): permissions is Permissions => {
	return (
		permissions !== undefined &&
		(permissions.viewDeploymentConfig ||
			permissions.viewAllLicenses ||
			permissions.viewAllUsers ||
			permissions.viewAnyGroup ||
			permissions.viewNotificationTemplate ||
			permissions.viewOrganizationIDPSyncSettings)
	);
};

/**
 * Checks if the user can view or edit members or groups for the organization
 * that produced the given OrganizationPermissions.
 */
export const canViewAnyOrganization = (
	permissions: Permissions | undefined,
): permissions is Permissions => {
	return (
		permissions !== undefined &&
		(permissions.viewAnyMembers ||
			permissions.editAnyGroups ||
			permissions.assignAnyRoles ||
			permissions.viewAnyIdpSyncSettings ||
			permissions.editAnySettings)
	);
};
