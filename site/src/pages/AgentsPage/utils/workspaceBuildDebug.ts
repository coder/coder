import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";

export const debugWorkspaceBuildPrompt = (build: WorkspaceBuild): string =>
	`Workspace ${build.workspace_owner_name}/${build.workspace_name} failed to ${build.transition}. Review the attached log information, determine why the workspace failed to ${build.transition}, and what resolving action the user can take. Respond with a 2-3 sentence summary of what the problem is and the action the user can take. Be concise, and keep it at a 15-year-old level. Do not start a new workspace.`;

// Owner and workspace names match UsernameValidRegex, so this is already a
// safe file name; the form sanitizes attachment names regardless.
export const debugWorkspaceBuildLogsFileName = (
	build: WorkspaceBuild,
): string =>
	`workspace-build-logs-${build.workspace_owner_name}-${build.workspace_name}-${build.build_number}.txt`;

// Same value as chatprompt.syntheticPasteInlineBudget, in bytes. chatd
// truncates only pasted-text files and sends other attachments uncut on every
// turn, so this trim bounds the prompt size.
/** @internal Exported for testing. */
export const debugWorkspaceBuildLogsMaxBytes = 128 * 1024;
// job.error has no server-side limit; Terraform puts the summary first.
const jobErrorMaxBytes = 8 * 1024;
const omittedMarker =
	"[earlier output omitted to fit the attachment size limit]\n";

const utf8 = new TextEncoder();
const byteLength = (text: string): number => utf8.encode(text).length;

// Truncates text to maxBytes of UTF-8, dropping a character split by the cut.
const truncateUtf8 = (
	text: string,
	maxBytes: number,
	{ keep }: { keep: "head" | "tail" },
): string => {
	if (keep === "head") {
		// encodeInto writes only whole characters.
		const { read } = utf8.encodeInto(text, new Uint8Array(maxBytes));
		return text.slice(0, read);
	}
	const bytes = utf8.encode(text);
	if (bytes.length <= maxBytes) {
		return text;
	}
	let start = bytes.length - maxBytes;
	// Skip continuation bytes so the tail starts on a character boundary.
	while ((bytes[start] & 0xc0) === 0x80) {
		start++;
	}
	return new TextDecoder().decode(bytes.subarray(start));
};

/**
 * Formats a build and its provisioner logs as the plain text chat attachment:
 * a header, then as much of the newest log output as fits within the byte
 * budget. The cut falls on a character boundary, so the first kept line may be
 * partial.
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
		const error = truncateUtf8(build.job.error, jobErrorMaxBytes, {
			keep: "head",
		});
		header.push(
			`Job error: ${error}${error === build.job.error ? "" : " (error truncated)"}`,
		);
	}
	if (build.job.logs_overflowed) {
		header.push(
			"Note: the build hit the provisioner log limit, so the end of the log is missing.",
		);
	}
	header.push("", "Build logs:");

	const lines: string[] = [];
	let stage: string | undefined;
	for (const log of logs) {
		if (log.stage !== stage) {
			stage = log.stage;
			lines.push("", `=== ${log.stage} (${log.created_at}) ===`);
		}
		lines.push(`[${log.log_level}] ${log.output}`);
	}
	if (logs.length === 0) {
		lines.push("(no build logs were recorded)");
	}

	const headerText = `${header.join("\n")}\n`;
	const body = `${lines.join("\n")}\n`;
	const bodyBudget = debugWorkspaceBuildLogsMaxBytes - byteLength(headerText);
	if (byteLength(body) <= bodyBudget) {
		return headerText + body;
	}
	return (
		headerText +
		omittedMarker +
		truncateUtf8(body, bodyBudget - byteLength(omittedMarker), {
			keep: "tail",
		})
	);
};
