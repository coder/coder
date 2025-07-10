import type { Workspace } from "api/typesGenerated";

// Returns true if the workspace is a prebuilt workspace (owned by the prebuilds system user),
// otherwise returns false.
export const isPrebuiltWorkspace = (workspace: Workspace): boolean => {
	return workspace.owner_id === "c42fdf75-3097-471c-8c33-fb52454d81c0";
};
