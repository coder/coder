export const createWorkspaceChecks = (organizationId: string) =>
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

export type CreateWSPermissions = Record<
  keyof ReturnType<typeof createWorkspaceChecks>,
  boolean
>;
