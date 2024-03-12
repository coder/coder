export const createWorkspaceChecks = (orgId: string) =>
  ({
    createWorkspaceForUser: {
      object: {
        resource_type: "workspace",
        organization_id: orgId,
        owner_id: "*",
      },
      action: "create",
    },
  }) as const;

export type CreateWSPermissions = Record<
  keyof ReturnType<typeof createWorkspaceChecks>,
  boolean
>;
