import type { AuthorizationCheck, Workspace } from "#/api/typesGenerated";
import type { ActionType } from "./actions";

export const workspaceChecks = (workspace: Workspace) =>
	({
		createWorkspace: {
			object: {
				resource_type: "workspace",
				organization_id: workspace.organization_id,
				owner_id: "me",
			},
			action: "create",
		},
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
		startWorkspace: {
			object: {
				resource_type: "workspace",
				resource_id: workspace.id,
				owner_id: workspace.owner_id,
			},
			action: "start",
		},
		stopWorkspace: {
			object: {
				resource_type: "workspace",
				resource_id: workspace.id,
				owner_id: workspace.owner_id,
			},
			action: "stop",
		},
		deleteWorkspace: {
			object: {
				resource_type: "workspace",
				resource_id: workspace.id,
				owner_id: workspace.owner_id,
			},
			action: "delete",
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

/** Whether the user can start the workspace with new parameters, restarting if necessary. */
export const canEditWorkspaceParameters = (
	workspace: Workspace,
	permissions: WorkspacePermissions | undefined,
): boolean =>
	Boolean(
		permissions?.startWorkspace &&
			(workspace.latest_build.status !== "running" ||
				permissions.stopWorkspace),
	);

/** Lists settings sections in navigation order with their access requirements evaluated. */
export const workspaceSettingsSections = (
	workspace: Workspace,
	permissions: WorkspacePermissions | undefined,
) => {
	const canUpdateWorkspace = permissions?.updateWorkspace ?? false;
	return [
		{ path: "", label: "General", allowed: canUpdateWorkspace },
		{
			path: "parameters",
			label: "Parameters",
			allowed: canEditWorkspaceParameters(workspace, permissions),
		},
		{
			path: "schedule",
			label: "Schedule",
			allowed: canUpdateWorkspace,
		},
		{
			path: "sharing",
			label: "Sharing",
			allowed: permissions?.shareWorkspace ?? false,
		},
	];
};

/** Checks transition permissions and allows display-only statuses once permissions load. */
export const canPerformWorkspaceAction = (
	workspace: Workspace,
	permissions: WorkspacePermissions | undefined,
	action: ActionType,
): boolean => {
	if (!permissions) return false;
	switch (action) {
		case "start":
		case "starting":
		case "updateAndStart":
		case "updateAndStartRequireActiveVersion":
			return permissions.startWorkspace;
		case "stop":
		case "stopping":
			return permissions.stopWorkspace;
		case "restart":
		case "restarting":
		case "updateAndRestart":
		case "updateAndRestartRequireActiveVersion":
			return permissions.startWorkspace && permissions.stopWorkspace;
		case "updating":
			return canEditWorkspaceParameters(workspace, permissions);
		case "activate":
		case "activating":
			return permissions.updateWorkspace;
		case "retry":
		case "debug":
			switch (workspace.latest_build.transition) {
				case "start":
					return permissions.startWorkspace;
				case "stop":
					return permissions.stopWorkspace;
				case "delete":
					return permissions.deleteWorkspace;
			}
			return false;
		case "deleting":
		case "canceling":
		case "deleted":
		case "pending":
			return true;
	}
};
