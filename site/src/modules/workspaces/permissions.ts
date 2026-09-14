import type { AuthorizationCheck, Workspace } from "#/api/typesGenerated";

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
		shareWorkspace: {
			object: {
				resource_type: "workspace",
				resource_id: workspace.id,
				owner_id: workspace.owner_id,
			},
			action: "share",
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
	}) satisfies Record<string, AuthorizationCheck>;

export type WorkspacePermissions = Record<
	keyof ReturnType<typeof workspaceChecks>,
	boolean
>;
