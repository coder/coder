export type OrganizationPermissions = {
	[k in OrganizationPermissionName]: boolean;
};

export type OrganizationPermissionName = keyof ReturnType<
	typeof organizationPermissionChecks
>;

export const organizationPermissionChecks = (organizationId: string) => ({
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
	editOrganization: {
		object: {
			resource_type: "organization",
			organization_id: organizationId,
		},
		action: "update",
	},
	assignOrgRole: {
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
	viewIdpSyncSettings: {
		object: {
			resource_type: "idpsync_settings",
			organization_id: organizationId,
		},
		action: "read",
	},
});
