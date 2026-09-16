import type * as TypesGen from "#/api/typesGenerated";

/** @internal Exported for testing. */
export const getWorkspaceOptionsWithLinkedWorkspace = (
	workspaceOptions: readonly TypesGen.Workspace[],
	workspace: TypesGen.Workspace | undefined,
	ownerID: string,
): readonly TypesGen.Workspace[] => {
	if (!workspace || workspace.owner_id !== ownerID) {
		return workspaceOptions;
	}

	const existingIndex = workspaceOptions.findIndex(
		(candidate) => candidate.id === workspace.id,
	);
	if (existingIndex === -1) {
		return [workspace, ...workspaceOptions];
	}

	if (workspaceOptions[existingIndex] === workspace) {
		return workspaceOptions;
	}

	const nextWorkspaceOptions = [...workspaceOptions];
	nextWorkspaceOptions[existingIndex] = workspace;
	return nextWorkspaceOptions;
};
