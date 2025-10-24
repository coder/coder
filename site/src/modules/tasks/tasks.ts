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

/**
 * Extracts a clean, human-readable task name from a workspace name.
 * Workspace names typically follow the pattern: task-{name}-{identifier}
 * This function removes the "task-" prefix and the identifier suffix,
 * leaving just the middle portion with dashes converted to spaces and title cased.
 *
 * Examples:
 * - "task-fix-login-bug-abc123" -> "Fix Login Bug"
 * - "task-add-feature-xyz789" -> "Add Feature"
 * - "simple-name" -> "Simple Name" (fallback for non-standard names)
 *
 * @param workspaceName - The full workspace name
 * @returns A cleaned, human-readable task name
 */
export function getCleanTaskName(workspaceName: string): string {
	// Remove "task-" prefix if present
	const cleaned = workspaceName.startsWith("task-")
		? workspaceName.slice(5)
		: workspaceName;

	// Split by dashes
	const parts = cleaned.split("-");

	// If we have multiple parts, remove the last one (likely an identifier)
	// Only do this if the last part looks like an identifier (short alphanumeric)
	if (parts.length > 2) {
		const lastPart = parts[parts.length - 1];
		// Check if last part is likely an identifier (short, contains numbers, or is a UUID-like string)
		if (
			lastPart.length <= 8 ||
			/\d/.test(lastPart) ||
			/^[a-f0-9]{8,}$/i.test(lastPart)
		) {
			parts.pop();
		}
	}

	// Join remaining parts with spaces and title case each word
	return parts
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase())
		.join(" ");
}
