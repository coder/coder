import type { WorkspacePermissions } from "#/modules/workspaces/permissions";

export const MockReadOnlyWorkspacePermissions: WorkspacePermissions = {
	createWorkspace: false,
	readWorkspace: true,
	startWorkspace: false,
	stopWorkspace: false,
	deleteWorkspace: false,
	updateWorkspace: false,
	shareWorkspace: false,
	updateWorkspaceVersion: false,
	deleteFailedWorkspace: false,
};
