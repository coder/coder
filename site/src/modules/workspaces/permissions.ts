import type { AuthorizationCheck, Workspace } from "api/typesGenerated";

export const workspaceChecks = (workspace: Workspace) =>
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
		updateWorkspaceVersion: {
			object: {
				resource_type: "template",
				resource_id: workspace.template_id,
			},
			action: "update",
		},
		// We only want to allow template admins to delete failed workspaces since
		// they can leave orphaned resources.
		deleteFailedWorkspace: {
			object: {
				resource_type: "template",
				resource_id: workspace.template_id,
			},
			action: "update",
		},
		// To run a build in debug mode we need to be able to read the deployment
		// config (enable_terraform_debug_mode).
		deploymentConfig: {
			object: {
				resource_type: "deployment_config",
			},
			action: "read",
		},
	}) satisfies Record<string, AuthorizationCheck>;

export type WorkspacePermissions = Record<
	keyof ReturnType<typeof workspaceChecks>,
	boolean
>;
