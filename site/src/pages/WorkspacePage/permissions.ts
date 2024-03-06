import type { Workspace, Template } from "api/typesGenerated";

export const workspaceChecks = (workspace: Workspace, template: Template) =>
  ({
    readWorkspace: {
      object: {
        resource_type: "workspace",
        resource_id: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "read",
    },
    updateWorkspace: {
      object: {
        resource_type: "workspace",
        resource_id: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "update",
    },
    updateTemplate: {
      object: {
        resource_type: "template",
        resource_id: template.id,
      },
      action: "update",
    },
    viewDeploymentValues: {
      object: {
        resource_type: "deployment_config",
      },
      action: "read",
    },
  }) as const;

export type WorkspacePermissions = Record<
  keyof ReturnType<typeof workspaceChecks>,
  boolean
>;
