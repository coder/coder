import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";

export const AI_PROMPT_PARAMETER_NAME = "AI Prompt";

export type Task = {
	workspace: Workspace;
	prompt: string;
};

export type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};

export function getTaskApps(task: Task): WorkspaceAppWithAgent[] {
	return task.workspace.latest_build.resources
		.flatMap((r) => r.agents ?? [])
		.flatMap((agent) =>
			agent.apps.map((app) => ({
				...app,
				agent,
			})),
		);
}
