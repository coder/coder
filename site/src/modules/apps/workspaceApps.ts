import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { findWorkspaceAgent, getWorkspaceAgents } from "#/utils/workspace";

export type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};

export function getAllAppsWithAgent(
	workspace: Workspace,
): WorkspaceAppWithAgent[] {
	return getWorkspaceAgents(workspace).flatMap((agent) =>
		agent.apps.map((app) => ({
			...app,
			agent,
		})),
	);
}

export function findWorkspaceAppWithAgent(
	workspace: Workspace,
	agentId: string,
	appId: string,
): WorkspaceAppWithAgent | undefined {
	const agent = findWorkspaceAgent(workspace, agentId);
	if (!agent) {
		return undefined;
	}
	const app = agent.apps.find((workspaceApp) => workspaceApp.id === appId);
	if (!app) {
		return undefined;
	}
	return { ...app, agent };
}
