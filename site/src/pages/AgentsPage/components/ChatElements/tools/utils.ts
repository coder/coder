import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import * as Diff from "diff";
import * as Yup from "yup";
import { asRecord, asString, isValid } from "../runtimeTypeUtils";

export type ToolStatus = "completed" | "error" | "running";

export interface EditFilesFileEntry {
	path: string;
	edits: Array<{ search: string; replace: string }>;
}

// Validates that the edit has at least the shape of an object with
// string-typed text fields. Accepts both current field names
// (old_text/new_text) and deprecated names (search/replace).
const normalizeEdit = (
	e: unknown,
): { search: string; replace: string } | null => {
	if (typeof e !== "object" || e === null) return null;
	const raw = e as Record<string, unknown>;
	const search =
		typeof raw.old_text === "string"
			? raw.old_text
			: typeof raw.search === "string"
				? raw.search
				: null;
	const replace =
		typeof raw.new_text === "string"
			? raw.new_text
			: typeof raw.replace === "string"
				? raw.replace
				: null;
	if (!search || replace === null) return null;
	return { search, replace };
};

const fileEntrySchema = Yup.object({
	path: Yup.string().required(),
	edits: Yup.array().defined(),
}).required();

type FileEntry = Yup.InferType<typeof fileEntrySchema>;

export const formatModelIntentLabel = (
	modelIntent: string | undefined,
): string => {
	const trimmed = modelIntent?.trim() ?? "";
	if (!trimmed) {
		return "";
	}
	return trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
};

const trailingDurationPattern =
	/(^|\s+)for\s+\d+(?:\.\d+)?\s*(?:ms|s|m|h)\s*$/i;

export const sanitizeExecuteModelIntent = (
	modelIntent: string | undefined,
	command: string,
): string => {
	const label = formatModelIntentLabel(modelIntent);
	const withoutCommand = stripRedundantUsingSuffix(label, command);
	return stripTrailingDuration(withoutCommand);
};

const stripRedundantUsingSuffix = (label: string, command: string): string => {
	const usingMatches = Array.from(label.matchAll(/(^|\s+)using\s+/gi));
	for (let i = usingMatches.length - 1; i >= 0; i--) {
		const match = usingMatches[i];
		if (match.index === undefined) {
			continue;
		}

		const suffix = stripTrailingDuration(
			label.slice(match.index + match[0].length),
		);
		if (isCommandReference(suffix, command)) {
			return label.slice(0, match.index).trim();
		}
	}
	return label;
};

const stripTrailingDuration = (label: string): string =>
	label.replace(trailingDurationPattern, "").trim();

const isCommandReference = (value: string, command: string): boolean => {
	const normalizedValue = normalizeCommandReference(value);
	const normalizedCommand = normalizeCommandReference(command);
	if (!normalizedValue || !normalizedCommand) {
		return false;
	}
	return (
		normalizedValue === normalizedCommand ||
		normalizedCommand.startsWith(`${normalizedValue} `) ||
		normalizedValue.startsWith(`${normalizedCommand} `)
	);
};

const normalizeCommandReference = (value: string): string =>
	value.trim().toLowerCase().replace(/\s+/g, " ");

export const toProviderLabel = (
	providerDisplayName: string,
	providerID: string,
	providerType: string,
): string => {
	if (providerDisplayName) {
		return providerDisplayName;
	}
	if (providerID) {
		return providerID;
	}
	if (providerType) {
		return providerType;
	}
	return "Git provider";
};

const roundToTenths = (value: number): number => Number(value.toFixed(1));

export const formatShellDurationMs = (
	durationMs: number | undefined,
): string => {
	if (
		durationMs === undefined ||
		durationMs < 0 ||
		!Number.isFinite(durationMs)
	) {
		return "";
	}
	if (durationMs < 1000) {
		return `${Math.round(durationMs)}ms`;
	}
	const seconds = roundToTenths(durationMs / 1000);
	if (seconds < 60) {
		return `${seconds}s`;
	}
	const minutes = roundToTenths(durationMs / 60_000);
	if (minutes < 60) {
		return `${minutes}m`;
	}
	const hours = roundToTenths(durationMs / 3_600_000);
	return `${hours}h`;
};

export const normalizeStatus = (status: string): string =>
	status.trim().toLowerCase();

export const isSubagentSuccessStatus = (status: string): boolean => {
	switch (normalizeStatus(status)) {
		case "completed":
		case "reported":
			return true;
		default:
			return false;
	}
};

export const isSubagentRunningStatus = (status: string): boolean => {
	switch (normalizeStatus(status)) {
		case "pending":
		case "running":
		case "awaiting":
			return true;
		default:
			return false;
	}
};

export const mapSubagentStatusToToolStatus = (
	subagentStatus: string,
	fallback: ToolStatus,
): ToolStatus => {
	const normalized = normalizeStatus(subagentStatus);
	if (!normalized) {
		return fallback;
	}
	if (isSubagentSuccessStatus(normalized)) {
		return "completed";
	}
	if (isSubagentRunningStatus(normalized)) {
		// If the tool call itself has already completed, don't
		// override to "running". The spawn/await tool is done;
		// the sub-agent may still be working in the background
		// but that doesn't mean the tool call is still running.
		return fallback === "completed" ? "completed" : "running";
	}
	switch (normalized) {
		case "waiting":
		case "terminated":
			return "completed";
		case "error":
			return "error";
		default:
			return fallback;
	}
};

export const parseArgs = (args: unknown): Record<string, unknown> | null => {
	if (!args) {
		return null;
	}
	if (typeof args === "string") {
		try {
			const parsed = JSON.parse(args);
			return asRecord(parsed);
		} catch {
			return null;
		}
	}
	return asRecord(args);
};

const getToolInputPayload = (args: unknown): unknown => {
	const rec = asRecord(args);
	if (!rec || typeof rec.model_intent !== "string") {
		return args;
	}
	if ("properties" in rec) {
		return rec.properties;
	}
	return Object.fromEntries(
		Object.entries(rec).filter(([key]) => key !== "model_intent"),
	);
};

const isEmptyObjectOrArray = (value: unknown): boolean => {
	if (Array.isArray(value)) {
		return value.length === 0;
	}
	const rec = asRecord(value);
	return rec ? Object.keys(rec).length === 0 : false;
};

const formatValue = (value: unknown): string => {
	if (typeof value === "object") {
		try {
			return JSON.stringify(value, null, 2) ?? String(value);
		} catch {
			return String(value);
		}
	}
	return String(value);
};

export const formatToolInput = (args: unknown): string | null => {
	const input = getToolInputPayload(args);
	if (input === undefined || input === null) {
		return null;
	}
	if (typeof input === "string") {
		const trimmed = input.trim();
		if (!trimmed) {
			return null;
		}
		try {
			return formatToolInput(JSON.parse(trimmed));
		} catch {
			return trimmed;
		}
	}
	return isEmptyObjectOrArray(input) ? null : formatValue(input);
};

export const formatResultOutput = (result: unknown): string | null => {
	if (result === undefined || result === null) {
		return null;
	}
	if (typeof result === "string") {
		const trimmed = result.trim();
		return trimmed || null;
	}
	const rec = asRecord(result);
	if (rec) {
		// For execute tool, show the output field.
		const output = asString(rec.output).trim();
		if (output) {
			return output;
		}
		// For read_file, show the content field.
		const content = asString(rec.content).trim();
		if (content) {
			return content;
		}
	}
	return formatValue(result);
};

export const fileViewerCSS = [
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }",
].join(" ");

// Restyled separators: quiet, full-width dividers that fade
// into the background instead of drawing attention.
export const SEPARATOR_CSS = [
	// Transparent backgrounds so separators blend with the
	// code area rather than forming a distinct band.
	":host {",
	"  --diffs-bg-separator-override: transparent;",
	"}",
	"[data-separator-content] {",
	"  border-radius: 0 !important;",
	"  background-color: transparent !important;",
	"}",
	"[data-separator-wrapper] {",
	"  border-radius: 0 !important;",
	"}",
	// Remove the inline padding that creates the inset pill look
	// so separators span the full width of the diff.
	"[data-unified] [data-separator='line-info'] [data-separator-wrapper] {",
	"  padding-inline: 0 !important;",
	"}",

	// Centered text with horizontal rules on either side:
	// ────── N unmodified lines ──────
	"[data-separator='line-info'] {",
	"  height: 28px !important;",
	"}",
	"[data-separator-content] {",
	"  display: flex !important;",
	"  align-items: center !important;",
	"  gap: 12px !important;",
	"  overflow: visible !important;",
	"  height: auto !important;",
	"  font-size: 11px !important;",
	"  color: hsl(var(--content-secondary)) !important;",
	"  opacity: 0.8;",
	"}",
	"[data-separator-content]::before,",
	"[data-separator-content]::after {",
	"  content: '' !important;",
	"  flex: 1 !important;",
	"  height: 1px !important;",
	"  background: hsl(var(--border-default)) !important;",
	"}",
].join(" ");

export const diffViewerCSS = [
	"pre, [data-line]:not([data-line-type='change-addition']):not([data-line-type='change-deletion']) { background-color: transparent !important; }",
	SEPARATOR_CSS,
].join(" ");

// Theme-aware option factories shared across tool renderers.
export function getDiffViewerOptions(isDark: boolean) {
	return {
		diffStyle: "unified" as const,
		diffIndicators: "bars" as const,
		overflow: "wrap" as const,
		themeType: (isDark ? "dark" : "light") as "dark" | "light",
		theme: isDark ? "github-dark-high-contrast" : "github-light",
		unsafeCSS: diffViewerCSS,
	};
}

/**
 * Returns a shallow clone of the diff with the no-EOF-newline
 * flags cleared on every hunk so the renderer never emits the
 * "No newline at end of file" row.  Use for inline tool diffs
 * where the indicator is visual noise.
 */
export function stripNoNewline(fileDiff: FileDiffMetadata): FileDiffMetadata {
	const needsStrip = fileDiff.hunks.some(
		(h) => h.noEOFCRDeletions || h.noEOFCRAdditions,
	);
	if (!needsStrip) return fileDiff;
	return {
		...fileDiff,
		hunks: fileDiff.hunks.map((h) => ({
			...h,
			noEOFCRDeletions: false,
			noEOFCRAdditions: false,
		})),
	};
}

export function getFileViewerOptions(isDark: boolean) {
	return {
		overflow: "scroll" as const,
		themeType: (isDark ? "dark" : "light") as "dark" | "light",
		theme: isDark ? "github-dark-high-contrast" : "github-light",
		unsafeCSS: fileViewerCSS,
	};
}

export function getFileViewerOptionsNoHeader(isDark: boolean) {
	return {
		...getFileViewerOptions(isDark),
		disableFileHeader: true,
	};
}

export function getFileViewerOptionsMinimal(isDark: boolean) {
	return {
		...getFileViewerOptions(isDark),
		disableFileHeader: true,
		disableLineNumbers: true,
	};
}

/**
 * Strips SVN-style "Index:" headers that `Diff.createPatch()`
 * emits but `@pierre/diffs` does not recognize as file
 * boundaries. Left in place they leak into hunk bodies and
 * trigger console errors.
 */
export const stripSvnIndexHeaders = (patch: string): string =>
	patch.replace(/^Index: .*\n={3,}\n/gm, "");

export const DIFFS_FONT_STYLE = {
	"--diffs-font-family": '"Geist Mono Variable", monospace, monospace',
	"--diffs-header-font-family": '"Geist Variable", system-ui, sans-serif',
	"--diffs-font-size": "11px",
	"--diffs-line-height": "1.5",
	"--diffs-addition-color-override": "hsl(var(--git-added))",
	"--diffs-deletion-color-override": "hsl(var(--git-deleted))",
	"--diffs-bg-addition-override": "hsl(var(--surface-git-added))",
	"--diffs-bg-deletion-override": "hsl(var(--surface-git-deleted))",
	"--diffs-bg-addition-number-override": "hsl(var(--surface-git-added))",
	"--diffs-bg-deletion-number-override": "hsl(var(--surface-git-deleted))",
	"--diffs-bg-selection-override": "hsl(var(--content-link) / 0.08)",
	"--diffs-bg-selection-number-override": "hsl(var(--content-link) / 0.13)",
	"--diffs-selection-number-fg": "hsl(var(--content-link))",
	"--diffs-gap-style": "1px solid hsl(var(--border-default))",
};

/**
 * Checks whether a tool result should be rendered as a syntax-highlighted
 * file viewer. Returns the file path, content, and whether the header
 * should be hidden.
 */
export const getFileContentForViewer = (
	toolName: string,
	args: unknown,
	result: unknown,
): {
	path: string;
	content: string;
	disableHeader?: boolean;
	disableLineNumbers?: boolean;
} | null => {
	if (toolName === "execute") {
		const rec = asRecord(result);
		if (!rec) {
			return null;
		}
		const output = asString(rec.output).trim();
		if (!output) {
			return null;
		}
		return {
			path: "output.sh",
			content: output,
			disableHeader: true,
			disableLineNumbers: true,
		};
	}
	if (toolName !== "read_file") {
		return null;
	}
	const parsed = parseArgs(args);
	const path = parsed ? asString(parsed.path).trim() : "";
	if (!path) {
		return null;
	}
	const rec = asRecord(result);
	if (!rec) {
		return null;
	}
	const content = asString(rec.content).trim();
	if (!content) {
		return null;
	}
	return { path, content };
};

/**
 * Parses a unified-diff string (with an optional SVN `Index:`
 * banner) into the first FileDiffMetadata it contains. Returns
 * null when the input is empty or the parser produces no files.
 * Shared between the write_file diff builder, the synthetic
 * edit_files diff builder, and the server-supplied diff parser.
 */
const parseSingleFileDiff = (raw: string): FileDiffMetadata | null => {
	if (!raw) return null;
	const parsed = parsePatchFiles(stripSvnIndexHeaders(raw));
	if (!parsed.length || !parsed[0].files.length) return null;
	return parsed[0].files[0];
};

/**
 * Builds a FileDiffMetadata representing a new-file diff (all lines
 * are additions) from the content written by a write_file tool call.
 * Returns null when the content is empty or unparsable.
 */
export const buildWriteFileDiff = (
	path: string,
	content: string,
): FileDiffMetadata | null => {
	if (!content) return null;
	return parseSingleFileDiff(Diff.createPatch(path, "", content, "", ""));
};

/**
 * For write_file tool calls, extracts the path and content from args
 * and builds a FileDiffMetadata showing all lines as additions.
 */
export const getWriteFileDiff = (
	toolName: string,
	args: unknown,
): FileDiffMetadata | null => {
	if (toolName !== "write_file") {
		return null;
	}
	const parsed = parseArgs(args);
	if (!parsed) {
		return null;
	}
	const path = asString(parsed.path).trim();
	const content = asString(parsed.content).trim();
	if (!path || !content) {
		return null;
	}
	return buildWriteFileDiff(path, content);
};

/** Height that fits roughly 3 lines of monospace text-xs output. */
export const COLLAPSED_OUTPUT_HEIGHT = 54;

/** Height for the collapsed report preview (~3 lines of rendered markdown). */
export const COLLAPSED_REPORT_HEIGHT = 72;

/**
 * Parses the args of an edit_files tool call into a typed array
 * of file entries.
 */
export const parseEditFilesArgs = (args: unknown): EditFilesFileEntry[] => {
	const parsed = parseArgs(args);
	if (!parsed) return [];
	const files = parsed.files;
	if (!Array.isArray(files)) return [];
	return files
		.filter((f): f is FileEntry => isValid(fileEntrySchema, f))
		.map((f) => ({
			path: f.path,
			edits: f.edits
				.map(normalizeEdit)
				.filter((e): e is { search: string; replace: string } => e !== null),
		}));
};

/**
 * Builds a synthetic unified diff from edit pairs (normalized to
 * search/replace) for a single file. Each edit becomes a separate
 * `Diff.createPatch` call; the patches are concatenated and
 * parsed into a single FileDiffMetadata.
 */
export const buildEditDiff = (
	path: string,
	edits: Array<{ search: string; replace: string }>,
): FileDiffMetadata | null => {
	if (!edits.length) return null;

	// Strip leading slash so the a/ and b/ prefixes don't
	// produce a double-slash that confuses the diff parser.
	const diffPath = path.startsWith("/") ? path.slice(1) : path;

	const patches: string[] = [];
	for (const edit of edits) {
		if (!edit.search) continue;
		patches.push(Diff.createPatch(diffPath, edit.search, edit.replace, "", ""));
	}
	if (!patches.length) {
		// All edits were skipped (empty search). Produce a
		// header-only patch so the parser still returns a file
		// entry with zero hunks.
		patches.push(`--- ${diffPath}\n+++ ${diffPath}\n`);
	}

	return parseSingleFileDiff(patches.join(""));
};

/**
 * Per-file result from the agent's FileEditResponse. `path` matches
 * the caller-supplied path (pre-symlink resolution). `diff` is a
 * unified-diff string, possibly empty for no-op edits.
 */
interface ServerEditResult {
	path: string;
	diff: string;
}

/**
 * Parses the structured `files` array from an edit_files tool
 * response. The field is only populated when the agent observed the
 * request's `include_diff` flag; older agents omit it entirely.
 * Returns null when no per-file result array is present (callers
 * should fall back to the synthetic client-side diff path). Returns
 * an empty array when the field is explicitly present but empty.
 */
export const parseServerEditResults = (
	result: unknown,
): ServerEditResult[] | null => {
	const rec = asRecord(result);
	if (!rec) return null;
	const raw = rec.files;
	if (raw === undefined || raw === null) return null;
	if (!Array.isArray(raw)) return null;
	const results: ServerEditResult[] = [];
	for (const entry of raw) {
		const entryRec = asRecord(entry);
		if (!entryRec) continue;
		const path = asString(entryRec.path).trim();
		if (!path) continue;
		results.push({ path, diff: asString(entryRec.diff) });
	}
	return results;
};

/**
 * Parses a server-supplied unified diff. Returns null for empty
 * strings (no-op edits) or when the parser produces no file entries.
 */
export const parseServerEditDiffText = (
	diff: string,
): FileDiffMetadata | null => parseSingleFileDiff(diff);

/**
 * Converts an MCP-prefixed tool name into a human-readable label.
 * E.g. "linear__list_issues" with slug "linear" → "List issues"
 */
export function humanizeMCPToolName(
	slug: string,
	prefixedName: string,
): string {
	const prefix = `${slug}__`;
	const raw = prefixedName.startsWith(prefix)
		? prefixedName.slice(prefix.length)
		: prefixedName;
	// Replace runs of underscores with a single space, then trim.
	const words = raw.replace(/_+/g, " ").trim();
	if (!words) {
		return prefixedName;
	}
	return words.charAt(0).toUpperCase() + words.slice(1);
}

// Re-export runtime type utils used by sub-components so they
// can import from a single location.
export { asNumber, asRecord, asString } from "../runtimeTypeUtils";

/**
 * Returns the tooltip label for a killed/terminated process signal.
 */
export const signalTooltipLabel = (signal: "kill" | "terminate"): string =>
	signal === "kill" ? "Killed (SIGKILL)" : "Terminated (SIGTERM)";

// Programs whose first positional argument is conventionally a subcommand verb.
const multiVerbTools = new Set([
	"git",
	"gh",
	"kubectl",
	"docker",
	"podman",
	"npm",
	"pnpm",
	"yarn",
	"go",
	"cargo",
	"make",
	"helm",
	"terraform",
	"systemctl",
	"brew",
]);

/**
 * Collapses parsed_commands into a comma-joined summary. Multi-verb
 * tools render as "<prog> <verb>"; others render as just "<prog>".
 * Consecutive duplicates are deduped.
 */
export const summarizeParsedCommands = (
	parsed: readonly string[][],
): string => {
	const labels: string[] = [];
	for (const entry of parsed) {
		const prog = entry[0];
		if (!prog) continue;
		const label =
			multiVerbTools.has(prog) && entry[1] ? `${prog} ${entry[1]}` : prog;
		if (labels[labels.length - 1] !== label) {
			labels.push(label);
		}
	}
	return labels.join(", ");
};
