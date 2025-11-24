export const createWorkspaceChecks = (
	organizationId: string,
	templateId?: string,
) =>
	({
		createWorkspaceForAny: {
			object: {
				resource_type: "workspace" as const,
				organization_id: organizationId,
				owner_id: "*",
			},
			action: "create" as const,
		},
		...(templateId && {
			canUpdateTemplate: {
				object: {
					resource_type: "template" as const,
					resource_id: templateId,
				},
				action: "update" as const,
			},
		}),
	}) as const;

export type CreateWorkspacePermissions = Record<
	keyof ReturnType<typeof createWorkspaceChecks>,
	boolean
>;
