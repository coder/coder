import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { isExternalApp } from "./apps";

export type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};

export function getWorkspaceAgents(workspace: Workspace): WorkspaceAgent[] {
	return workspace.latest_build.resources.flatMap(
		(resource) => resource.agents ?? [],
	);
}

export function findWorkspaceAgent(
	workspace: Workspace,
	agentId: string,
): WorkspaceAgent | undefined {
	return getWorkspaceAgents(workspace).find((agent) => agent.id === agentId);
}

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

/**
 * True for apps that can be rendered inside a dashboard iframe. Command apps
 * open in terminal tabs instead.
 */
export function isWorkspaceAppEmbeddable(app: WorkspaceApp): boolean {
	return !app.hidden && !isExternalApp(app) && !app.command;
}

/**
 * True when an app requires subdomain access but the deployment has no wildcard
 * access URL configured, so the app cannot be launched or embedded.
 */
export function isAppBlockedByMissingWildcard(
	app: WorkspaceApp,
	wildcardHostname: string | undefined,
): boolean {
	return app.subdomain && !wildcardHostname;
}
