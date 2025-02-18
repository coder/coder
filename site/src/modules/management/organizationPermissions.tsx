import type { AuthorizationCheck } from "api/typesGenerated";

export type OrganizationPermissions = {
	[k in OrganizationPermissionName]: boolean;
};

export type OrganizationPermissionName = keyof ReturnType<
	typeof organizationPermissionChecks
>;

export const organizationPermissionChecks = (organizationId: string) =>
	({
		viewMembers: {
			object: {
				resource_type: "organization_member",
				organization_id: organizationId,
			},
			action: "read",
		},
		editMembers: {
			object: {
				resource_type: "organization_member",
				organization_id: organizationId,
			},
			action: "update",
		},
		createGroup: {
			object: {
				resource_type: "group",
				organization_id: organizationId,
			},
			action: "create",
		},
		viewGroups: {
			object: {
				resource_type: "group",
				organization_id: organizationId,
			},
			action: "read",
		},
		editGroups: {
			object: {
				resource_type: "group",
				organization_id: organizationId,
			},
			action: "update",
		},
		editSettings: {
			object: {
				resource_type: "organization",
				organization_id: organizationId,
			},
			action: "update",
		},
		assignOrgRoles: {
			object: {
				resource_type: "assign_org_role",
				organization_id: organizationId,
			},
			action: "assign",
		},
		viewOrgRoles: {
			object: {
				resource_type: "assign_org_role",
				organization_id: organizationId,
			},
			action: "read",
		},
		createOrgRoles: {
			object: {
				resource_type: "assign_org_role",
				organization_id: organizationId,
			},
			action: "create",
		},
		viewProvisioners: {
			object: {
				resource_type: "provisioner_daemon",
				organization_id: organizationId,
			},
			action: "read",
		},
		viewProvisionerJobs: {
			object: {
				resource_type: "provisioner_jobs",
				organization_id: organizationId,
			},
			action: "read",
		},
		viewIdpSyncSettings: {
			object: {
				resource_type: "idpsync_settings",
				organization_id: organizationId,
			},
			action: "read",
		},
		editIdpSyncSettings: {
			object: {
				resource_type: "idpsync_settings",
				organization_id: organizationId,
			},
			action: "update",
		},
	}) as const satisfies Record<string, AuthorizationCheck>;

/**
 * Checks if the user can view or edit members or groups for the organization
 * that produced the given OrganizationPermissions.
 */
export const canViewOrganization = (
	permissions: OrganizationPermissions | undefined,
): permissions is OrganizationPermissions => {
	return (
		permissions !== undefined &&
		(permissions.viewMembers ||
			permissions.viewGroups ||
			permissions.viewOrgRoles ||
			permissions.viewProvisioners ||
			permissions.viewIdpSyncSettings)
	);
};

/**
 * Return true if the user can edit the organization settings or its members.
 */
export const canEditOrganization = (
	permissions: OrganizationPermissions | undefined,
): permissions is OrganizationPermissions => {
	return (
		permissions !== undefined &&
		(permissions.editMembers ||
			permissions.editGroups ||
			permissions.editSettings ||
			permissions.assignOrgRoles ||
			permissions.editIdpSyncSettings ||
			permissions.createOrgRoles)
	);
};

export type AnyOrganizationPermissions = {
	[k in AnyOrganizationPermissionName]: boolean;
};

export type AnyOrganizationPermissionName =
	keyof typeof anyOrganizationPermissionChecks;

export const anyOrganizationPermissionChecks = {
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
} as const satisfies Record<string, AuthorizationCheck>;

/**
 * Checks if the user can view or edit members or groups for the organization
 * that produced the given OrganizationPermissions.
 */
export const canViewAnyOrganization = (
	permissions: AnyOrganizationPermissions | undefined,
): permissions is AnyOrganizationPermissions => {
	return (
		permissions !== undefined &&
		(permissions.viewAnyMembers ||
			permissions.editAnyGroups ||
			permissions.assignAnyRoles ||
			permissions.viewAnyIdpSyncSettings ||
			permissions.editAnySettings)
	);
};
