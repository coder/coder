import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";

export type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};

export function getAllAppsWithAgent(
	workspace: Workspace,
): WorkspaceAppWithAgent[] {
	return workspace.latest_build.resources
		.flatMap((r) => r.agents ?? [])
		.flatMap((agent) =>
			agent.apps.map((app) => ({
				...app,
				agent,
			})),
		);
}
