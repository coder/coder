import type { ProvisionerJobLog, WorkspaceBuild } from "#/api/typesGenerated";

export const debugWorkspaceBuildPrompt = (build: WorkspaceBuild): string =>
	`Workspace ${build.workspace_owner_name}/${build.workspace_name} failed to ${build.transition}. Review the attached log information, determine why the workspace failed to ${build.transition}, and what resolving action the user can take. Respond with a 2-3 sentence summary of what the problem is and the action the user can take. Be concise, and keep it at a 15-year-old level. Do not start a new workspace.`;

// Owner and workspace names match UsernameValidRegex, so this is already a
// safe file name; the form sanitizes attachment names regardless.
export const debugWorkspaceBuildLogsFileName = (
	build: WorkspaceBuild,
): string =>
	`workspace-build-logs-${build.workspace_owner_name}-${build.workspace_name}-${build.build_number}.txt`;

// Same value as chatprompt.syntheticPasteInlineBudget, in bytes. chatd applies
// that budget only to pasted-text files, so this trim is the only cap on the
// attachment, which is replayed on every turn.
/** @internal Exported for testing. */
export const debugWorkspaceBuildLogsMaxBytes = 128 * 1024;
// Room for the omission marker and a re-emitted stage header.
const markerReserveBytes = 512;
// job.error has no server-side limit; Terraform puts the summary first.
const jobErrorMaxBytes = 8 * 1024;

const utf8 = new TextEncoder();
// Includes the newline that joins the lines.
const lineBytes = (line: string): number => utf8.encode(line).length + 1;

const linesBytes = (lines: readonly string[]): number =>
	lines.reduce((total, line) => total + lineBytes(line), 0);

// Truncates text to maxBytes of UTF-8, dropping a character split by the cut.
/** @internal Exported for testing. */
export const truncateUtf8 = (
	text: string,
	maxBytes: number,
	keep: "head" | "tail",
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

type LogLine = {
	text: string;
	// Set only on log output: labels a trimmed block and marks the line as
	// counted in the omitted total.
	stageHeader?: string;
};

/**
 * Formats a build and its provisioner logs as the plain text chat attachment.
 * Keeps the header and the most recent log lines within the byte budget.
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
		const error = truncateUtf8(build.job.error, jobErrorMaxBytes, "head");
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

	const lines: LogLine[] = [];
	let currentStage: string | undefined;
	let stageHeader: string | undefined;
	for (const log of logs) {
		if (log.stage !== currentStage) {
			currentStage = log.stage;
			stageHeader = `=== ${log.stage} (${log.created_at}) ===`;
			lines.push({ text: "" }, { text: stageHeader });
		}
		lines.push({ text: `[${log.log_level}] ${log.output}`, stageHeader });
	}
	if (logs.length === 0) {
		lines.push({ text: "(no build logs were recorded)" });
	}

	const headerBytes = linesBytes(header);
	const texts = lines.map((line) => line.text);
	if (headerBytes + linesBytes(texts) <= debugWorkspaceBuildLogsMaxBytes) {
		return `${[...header, ...texts].join("\n")}\n`;
	}

	const lineBudget =
		debugWorkspaceBuildLogsMaxBytes - headerBytes - markerReserveBytes;
	let start = lines.length;
	let used = 0;
	while (start > 0 && used + lineBytes(texts[start - 1]) <= lineBudget) {
		used += lineBytes(texts[start - 1]);
		start--;
	}
	// Even the last line alone can be over budget; keep its tail then.
	const truncateLast = start === lines.length;
	if (truncateLast) {
		start = lines.length - 1;
	}
	const first = lines[start];
	const omitted = lines
		.slice(0, start)
		.filter((line) => line.stageHeader).length;
	const markers: string[] = [];
	if (omitted > 0) {
		markers.push(
			`[${omitted} earlier lines omitted to fit the attachment size limit]`,
		);
	}
	if (first.stageHeader) {
		markers.push(first.stageHeader);
	}
	let kept = texts.slice(start);
	if (truncateLast) {
		markers.push(
			"[the start of the next line was omitted to fit the attachment size limit]",
		);
		kept = [
			truncateUtf8(
				first.text,
				Math.max(lineBudget - linesBytes(markers), 0),
				"tail",
			),
		];
	}
	return `${[...header, ...markers, ...kept].join("\n")}\n`;
};
