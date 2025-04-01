export const workspacePermissionChecks = (organizationId: string) =>
	({
		createWorkspaceForUser: {
			object: {
				resource_type: "workspace",
				organization_id: organizationId,
				owner_id: "*",
			},
			action: "create",
		},
	}) as const;

export type WorkspacePermissions = Record<
	keyof ReturnType<typeof workspacePermissionChecks>,
	boolean
>;

export type WorkspacePermissionName = keyof ReturnType<
	typeof workspacePermissionChecks
>;
