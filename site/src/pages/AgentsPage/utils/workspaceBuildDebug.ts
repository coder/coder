import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { sanitizeChatFileName } from "./chatAttachments";

/**
 * Search param on `/agents` that asks the create page to open a chat about a
 * failed workspace build. The value is the workspace build ID; the page fetches
 * the build and its logs itself so the link stays short and shareable.
 */
export const debugWorkspaceBuildSearchParam = "debug_workspace_build";

export const debugWorkspaceBuildPrompt =
	"This workspace failed to startup. Review the attached log information, determine why the workspace failed to start, and what resolving action the user can take. Respond with a 2-3 sentence summary of what the problem is and action the user can take. Be concise, and keep it at a 15 years old level. Do not start a new workspace.";

export const buildDebugWorkspaceBuildPath = (buildId: string): string =>
	`/agents?${debugWorkspaceBuildSearchParam}=${encodeURIComponent(buildId)}`;

export const debugWorkspaceBuildLogsFileName = (
	build: WorkspaceBuild,
): string =>
	sanitizeChatFileName(
		`workspace-build-logs-${build.workspace_owner_name}-${build.workspace_name}-${build.build_number}.txt`,
	);

/**
 * Renders a build and its provisioner logs as plain text for a chat
 * attachment. Logs are grouped by stage, matching the build logs UI.
 */
export const formatWorkspaceBuildLogsForDebug = (
	build: WorkspaceBuild,
	logs: readonly ProvisionerJobLog[],
): string => {
	const lines = [
		`Workspace: ${build.workspace_owner_name}/${build.workspace_name}`,
		`Template version: ${build.template_version_name}`,
		`Build: #${build.build_number} (${build.transition}, reason: ${build.reason})`,
		`Build status: ${build.status}`,
		`Job status: ${build.job.status}`,
	];
	if (build.job.error_code) {
		lines.push(`Job error code: ${build.job.error_code}`);
	}
	if (build.job.error) {
		lines.push(`Job error: ${build.job.error}`);
	}
	lines.push("", "Build logs:");

	let currentStage: string | undefined;
	for (const log of logs) {
		if (log.stage !== currentStage) {
			currentStage = log.stage;
			lines.push("", `=== ${log.stage} ===`);
		}
		lines.push(`${log.created_at} [${log.log_level}] ${log.output}`);
	}
	if (logs.length === 0) {
		lines.push("(no build logs were recorded)");
	}

	return `${lines.join("\n")}\n`;
};
