import type { WorkspaceAgent, WorkspaceApp } from "api/typesGenerated";

export type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};
