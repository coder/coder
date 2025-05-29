import type { Workspace } from "api/typesGenerated";

export const AI_PROMPT_PARAMETER_NAME = "AI Prompt";

export type Task = {
	workspace: Workspace;
	prompt: string;
};
