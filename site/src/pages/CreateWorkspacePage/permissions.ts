export const createWorkspaceChecks = (organizationId: string) =>
	({
		createWorkspaceForAny: {
			object: {
				resource_type: "workspace",
				organization_id: organizationId,
				owner_id: "*",
			},
			action: "create",
		},
	}) as const;

export type CreateWorkspacePermissions = Record<
	keyof ReturnType<typeof createWorkspaceChecks>,
	boolean
>;
