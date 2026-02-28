import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import type React from "react";
import { asRecord, asString } from "../runtimeTypeUtils";

export type ToolStatus = "completed" | "error" | "running";

export interface EditFilesFileEntry {
	path: string;
	edits: Array<{ search: string; replace: string }>;
}

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

/**
 * Formats a duration in milliseconds into a compact label using
 * the same style as {@link shortRelativeTime} in utils/time.
 */
export const shortDurationMs = (durationMs: number | undefined): string => {
	if (durationMs === undefined || durationMs < 0) {
		return "";
	}
	const seconds = Math.round(durationMs / 1000);
	if (seconds < 60) {
		return `${seconds}s`;
	}
	const minutes = Math.round(seconds / 60);
	if (minutes < 60) {
		return `${minutes}m`;
	}
	const hours = Math.round(minutes / 60);
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
	if (typeof result === "object") {
		try {
			return JSON.stringify(result, null, 2);
		} catch {
			return String(result);
		}
	}
	return String(result);
};

export const fileViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; }";

export const diffViewerCSS =
	"pre, [data-line], [data-diffs-header] { background-color: transparent !important; } [data-diffs-header] { border-left: 1px solid var(--border); }";

// Theme-aware option factories shared across tool renderers.
export function getDiffViewerOptions(isDark: boolean) {
	return {
		diffStyle: "unified" as const,
		diffIndicators: "bars" as const,
		overflow: "scroll" as const,
		themeType: (isDark ? "dark" : "light") as "dark" | "light",
		theme: isDark ? "github-dark-high-contrast" : "github-light",
		unsafeCSS: diffViewerCSS,
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

export const DIFFS_FONT_STYLE = {
	"--diffs-font-size": "11px",
	"--diffs-line-height": "1.5",
} as React.CSSProperties;

export const BORDER_BG_STYLE = {
	background: "hsl(var(--border-default))",
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
 * Builds a FileDiffMetadata representing a new-file diff (all lines
 * are additions) from the content written by a write_file tool call.
 * Returns null when the content is empty or unparsable.
 */
export const buildWriteFileDiff = (
	path: string,
	content: string,
): FileDiffMetadata | null => {
	const lines = content.split("\n");
	// Remove trailing empty line produced by a final newline.
	if (lines.length > 0 && lines[lines.length - 1] === "") {
		lines.pop();
	}
	if (lines.length === 0) {
		return null;
	}

	const patchLines = [
		`diff --git a/${path} b/${path}`,
		"new file mode 100644",
		"--- /dev/null",
		`+++ b/${path}`,
		`@@ -0,0 +1,${lines.length} @@`,
		...lines.map((l) => `+${l}`),
	];
	const patch = `${patchLines.join("\n")}\n`;

	const parsed = parsePatchFiles(patch);
	if (!parsed.length || !parsed[0].files.length) {
		return null;
	}
	return parsed[0].files[0];
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
	return files.filter(
		(f): f is EditFilesFileEntry =>
			f !== null &&
			typeof f === "object" &&
			typeof (f as Record<string, unknown>).path === "string" &&
			Array.isArray((f as Record<string, unknown>).edits),
	);
};

/**
 * Builds a synthetic unified diff from search/replace edit pairs
 * for a single file. Each pair becomes a separate hunk in the
 * diff. Line numbers are synthetic since we don't have the full
 * file content.
 */
export const buildEditDiff = (
	path: string,
	edits: Array<{ search: string; replace: string }>,
): FileDiffMetadata | null => {
	if (!edits.length) return null;

	// Strip leading slash so the a/ and b/ prefixes don't
	// produce a double-slash that confuses the diff parser.
	const diffPath = path.startsWith("/") ? path.slice(1) : path;

	const patchLines: string[] = [
		`diff --git a/${diffPath} b/${diffPath}`,
		`--- a/${diffPath}`,
		`+++ b/${diffPath}`,
	];

	let lineOffset = 1;
	for (const edit of edits) {
		if (!edit.search) continue;
		const searchLines = edit.search.split("\n");
		const replaceLines = edit.replace.split("\n");

		// Remove trailing empty line produced by a final newline.
		if (searchLines.length > 0 && searchLines[searchLines.length - 1] === "") {
			searchLines.pop();
		}
		if (
			replaceLines.length > 0 &&
			replaceLines[replaceLines.length - 1] === ""
		) {
			replaceLines.pop();
		}
		if (searchLines.length === 0 && replaceLines.length === 0) continue;

		patchLines.push(
			`@@ -${lineOffset},${searchLines.length} +${lineOffset},${replaceLines.length} @@`,
		);
		for (const l of searchLines) patchLines.push(`-${l}`);
		for (const l of replaceLines) patchLines.push(`+${l}`);

		lineOffset += Math.max(searchLines.length, replaceLines.length) + 1;
	}

	const patch = `${patchLines.join("\n")}\n`;
	const parsed = parsePatchFiles(patch);
	if (!parsed.length || !parsed[0].files.length) return null;
	return parsed[0].files[0];
};

// Re-export runtime type utils used by sub-components so they
// can import from a single location.
export { asNumber, asRecord, asString } from "../runtimeTypeUtils";
