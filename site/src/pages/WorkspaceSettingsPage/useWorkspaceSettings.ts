import { createContext, useContext } from "react";
import type { Workspace } from "#/api/typesGenerated";
import type { WorkspacePermissions } from "#/modules/workspaces/permissions";

type WorkspaceSettingsContext = {
	owner: string;
	workspace: Workspace;
	permissions?: WorkspacePermissions;
};

export const WorkspaceSettings = createContext<
	WorkspaceSettingsContext | undefined
>(undefined);

export function useWorkspaceSettings() {
	const value = useContext(WorkspaceSettings);
	if (!value) {
		throw new Error(
			"This hook can only be used from a workspace settings page",
		);
	}

	return value;
}
