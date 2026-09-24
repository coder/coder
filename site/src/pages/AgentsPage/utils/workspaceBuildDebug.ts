import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";
import { sanitizeChatFileName } from "./chatAttachments";

// Failed build ID; the create page refetches the build and logs so the link
// stays shareable.
export const debugWorkspaceBuildSearchParam = "debug_workspace_build";

// Written by the workspace page when the user clicks the action and cleared
// by the create page on mount, so only a real click sends without
// confirmation. A pasted or replayed link prefills the chat instead.
const debugWorkspaceBuildIntentStorageKey =
	"agents.debug-workspace-build-intent";
const debugWorkspaceBuildIntentMaxAgeMs = 5 * 60 * 1000;

export const storeDebugWorkspaceBuildIntent = (buildId: string): void => {
	localStorage.setItem(
		debugWorkspaceBuildIntentStorageKey,
		JSON.stringify({ buildId, at: Date.now() }),
	);
};

export const clearDebugWorkspaceBuildIntent = (): void => {
	localStorage.removeItem(debugWorkspaceBuildIntentStorageKey);
};

export const hasDebugWorkspaceBuildIntent = (buildId: string): boolean => {
	const raw = localStorage.getItem(debugWorkspaceBuildIntentStorageKey);
	if (raw === null) {
		return false;
	}
	try {
		const intent: unknown = JSON.parse(raw);
		return (
			typeof intent === "object" &&
			intent !== null &&
			"buildId" in intent &&
			intent.buildId === buildId &&
			"at" in intent &&
			typeof intent.at === "number" &&
			Date.now() - intent.at < debugWorkspaceBuildIntentMaxAgeMs
		);
	} catch {
		return false;
	}
};

export const debugWorkspaceBuildPrompt = (build: WorkspaceBuild): string =>
	`Workspace ${build.workspace_owner_name}/${build.workspace_name} failed to ${build.transition}. Review the attached log information, determine why the workspace failed to ${build.transition}, and what resolving action the user can take. Respond with a 2-3 sentence summary of what the problem is and the action the user can take. Be concise, and keep it at a 15-year-old level. Do not start a new workspace.`;

export const buildDebugWorkspaceBuildPath = (buildId: string): string =>
	`/agents?${debugWorkspaceBuildSearchParam}=${encodeURIComponent(buildId)}`;

export const debugWorkspaceBuildLogsFileName = (
	build: WorkspaceBuild,
): string =>
	sanitizeChatFileName(
		`workspace-build-logs-${build.workspace_owner_name}-${build.workspace_name}-${build.build_number}.txt`,
	);

// Matches the inline budget chatd applies to pasted text. The attachment is
// replayed on every turn, so anything larger is pure token cost.
export const debugWorkspaceBuildLogsMaxChars = 128 * 1024;

/**
 * Formats a build and its provisioner logs as the plain text chat attachment.
 * Keeps the header and the most recent log lines within the size budget.
 */
export const formatWorkspaceBuildLogsForDebug = (
	build: WorkspaceBuild,
	logs: readonly ProvisionerJobLog[],
): string => {
	const header = [
		`Workspace: ${build.workspace_owner_name}/${build.workspace_name}`,
		`Template version: ${build.template_version_name}`,
		`Build: #${build.build_number} (${build.transition}, reason: ${build.reason})`,
		`Build status: ${build.status}`,
	];
	if (build.job.error_code) {
		header.push(`Job error code: ${build.job.error_code}`);
	}
	if (build.job.error) {
		header.push(`Job error: ${build.job.error}`);
	}
	if (build.job.logs_overflowed) {
		header.push(
			"Note: the build hit the provisioner log limit, so the end of the log is missing.",
		);
	}
	header.push("", "Build logs:");

	const logLines: string[] = [];
	let currentStage: string | undefined;
	for (const log of logs) {
		if (log.stage !== currentStage) {
			currentStage = log.stage;
			logLines.push("", `=== ${log.stage} (${log.created_at}) ===`);
		}
		logLines.push(`[${log.log_level}] ${log.output}`);
	}
	if (logs.length === 0) {
		logLines.push("(no build logs were recorded)");
	}

	let size = [...header, ...logLines].reduce(
		(total, line) => total + line.length + 1,
		0,
	);
	let omitted = 0;
	while (size > debugWorkspaceBuildLogsMaxChars && logLines.length > 1) {
		const dropped = logLines.shift() ?? "";
		size -= dropped.length + 1;
		omitted++;
	}
	if (omitted > 0) {
		logLines.unshift(
			`[${omitted} earlier lines omitted to fit the attachment size limit]`,
		);
	}

	return `${[...header, ...logLines].join("\n")}\n`;
};
