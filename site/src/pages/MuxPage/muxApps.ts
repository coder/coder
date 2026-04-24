import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";

export type MuxAppCandidate = {
	agent: WorkspaceAgent;
	app: WorkspaceApp;
};

const isMuxApp = (app: WorkspaceApp): boolean => {
	return app.slug === "mux" || app.icon === "/icon/mux.svg";
};

export const getMuxCandidatesFromWorkspace = (
	workspace: Workspace,
): MuxAppCandidate[] => {
	const candidates: MuxAppCandidate[] = [];

	for (const resource of workspace.latest_build.resources) {
		for (const agent of resource.agents ?? []) {
			for (const app of agent.apps) {
				if (isMuxApp(app)) {
					candidates.push({ agent, app });
				}
			}
		}
	}

	return candidates;
};

export const pickPreferredMuxApp = (
	candidates: readonly MuxAppCandidate[],
): MuxAppCandidate | undefined => {
	return (
		candidates.find(({ app }) => app.slug === "mux") ??
		candidates
			.toSorted((left, right) => left.app.slug.localeCompare(right.app.slug))
			.at(0)
	);
};

export const filterMuxWorkspaces = (
	workspaces: readonly Workspace[],
): Workspace[] => {
	return workspaces.filter((workspace) => {
		const status = workspace.latest_build.status;
		const isRunnableMuxWorkspace = status === "running" || status === "stopped";

		return (
			isRunnableMuxWorkspace &&
			workspace.dormant_at === null &&
			!workspace.is_prebuild &&
			getMuxCandidatesFromWorkspace(workspace).length > 0
		);
	});
};
