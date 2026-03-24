import type { FileDiffMetadata } from "@pierre/diffs";
import { parsePatchFiles } from "@pierre/diffs";
import * as Diff from "diff";
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

// Selection override CSS maps the library's gold/yellow selection
// palette to the Coder blue accent (`--content-link`) so line
// highlighting feels native to the rest of the page.
//
// The library has two selection code paths: context lines use
// `--diffs-bg-selection`, but change-addition/deletion lines
// use a separate `color-mix()` against `--diffs-line-bg`. To
// guarantee a uniform highlight across all line types we set
// the CSS variables for annotations AND apply direct rules
// with `!important` for line and gutter elements.
const SELECTION_OVERRIDE_CSS = [
	// Variable overrides for annotation areas and library internals.
	":host {",
	"  --diffs-bg-selection-override: hsl(var(--content-link) / 0.08);",
	"  --diffs-bg-selection-number-override: hsl(var(--content-link) / 0.13);",
	"  --diffs-selection-number-fg: hsl(var(--content-link));",
	"  --diffs-gap-style: 1px solid hsl(var(--border-default));",
	"}",
	// Direct rules that override both context and change-line
	// selection backgrounds so every selected line looks the same.
	"[data-selected-line][data-line] {",
	"  background-color: hsl(var(--content-link) / 0.08) !important;",
	"}",
	"[data-selected-line][data-column-number] {",
	"  background-color: hsl(var(--content-link) / 0.13) !important;",
	"  color: hsl(var(--content-link)) !important;",
	"}",
	// Clear the selection tint from annotation rows so the inline
	// prompt input stands out clearly against the selected lines.
	"[data-line-annotation][data-selected-line] [data-annotation-content] {",
	"  background-color: transparent !important;",
	"}",
	"[data-line-annotation][data-selected-line]::before {",
	"  background-color: transparent !important;",
	"}",
	"[data-selected-line][data-gutter-buffer='annotation'] {",
	"  background-color: transparent !important;",
	"}",
].join(" ");

// Restyled separators: quiet, full-width dividers that fade
// into the background instead of drawing attention.
const SEPARATOR_CSS = [
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

// Shared header styling applied to all diff viewers (both the
// conversation-inline diffs and the right-tab panel). This gives
// every diff header the same font sizing, change-type badges,
// and stat-count pills regardless of where it appears.
const DIFF_HEADER_CSS = [
	// Header layout: consistent sizing and padding across contexts.
	"[data-diffs-header] {",
	"  font-size: 13px;",
	"  min-height: 32px !important;",
	"  padding-block: 8px !important;",
	"  padding-inline: 10px 6px !important;",
	"  border-bottom: 1px solid hsl(var(--border-default));",
	"}",

	// Title text: sans-serif, slightly smaller than header chrome.
	"[data-diffs-header] [data-title] {",
	"  font-size: 12px;",
	"  color: hsl(var(--content-primary));",
	"}",

	// Replace the library's built-in SVG change-type icons with
	// single-letter badges (A/D/M/R) via CSS-generated content.
	"[data-change-icon] { display: none !important; }",
	// Baseline-align the badge letter with the filename so their
	// text baselines match despite different font sizes (11px vs
	// 12px). Without this the box-centering default shifts the
	// badge a fraction of a pixel above the title.
	"[data-diffs-header] [data-header-content] { align-items: baseline; overflow: hidden; }",
	"[data-diffs-header] [data-rename-icon] { align-self: center; }",
	"[data-diffs-header] [data-header-content]::before {",
	"  font-size: 11px;",
	"  font-weight: 600;",
	"  flex-shrink: 0;",
	"}",
	"[data-diffs-header][data-change-type='new'] [data-header-content]::before {",
	"  content: 'A';",
	"  color: hsl(var(--git-added));",
	"}",
	"[data-diffs-header][data-change-type='change'] [data-header-content]::before {",
	"  content: 'M';",
	"  color: hsl(var(--git-modified));",
	"}",
	"[data-diffs-header][data-change-type='deleted'] [data-header-content]::before {",
	"  content: 'D';",
	"  color: hsl(var(--git-deleted));",
	"}",
	"[data-diffs-header][data-change-type='rename-pure'] [data-header-content]::before,",
	"[data-diffs-header][data-change-type='rename-changed'] [data-header-content]::before {",
	"  content: 'R';",
	"  color: hsl(var(--git-modified));",
	"}",

	// Stat counts styled as compact pill badges.
	"[data-diffs-header] [data-metadata] {",
	"  flex-shrink: 0;",
	"  flex-direction: row-reverse;",
	"  align-items: stretch;",
	"  gap: 0 !important;",
	"  padding: 0;",
	"  border: 1px solid hsl(var(--border-default));",
	"  border-radius: 3px;",
	"  overflow: hidden;",
	"}",
	"[data-diffs-header] [data-additions-count],",
	"[data-diffs-header] [data-deletions-count] {",
	"  font-family: var(--diffs-font-family, var(--diffs-font-fallback));",
	"  font-size: 12px;",
	"  font-weight: 500;",
	"  line-height: 20px;",
	"  padding-inline: 4px;",
	"  border-radius: 0;",
	"}",
	"[data-diffs-header] [data-additions-count] {",
	"  color: hsl(var(--git-added-bright)) !important;",
	"  background-color: hsl(var(--surface-git-added));",
	"}",
	"[data-diffs-header] [data-deletions-count] {",
	"  color: hsl(var(--git-deleted-bright)) !important;",
	"  background-color: hsl(var(--surface-git-deleted));",
	"}",
].join(" ");

export const diffViewerCSS = [
	// Make context lines transparent so they blend with the page,
	// but preserve the library's colored backgrounds on changed
	// lines (change-addition / change-deletion) so the line-level
	// tint and word-level emphasis highlights remain visible.
	"pre, [data-line]:not([data-selected-line]):not([data-line-type='change-addition']):not([data-line-type='change-deletion']), [data-diffs-header] { background-color: transparent !important; }",
	"[data-diffs-header] { border-left: 1px solid var(--border); }",
	// The library reserves a 6 px horizontal scrollbar track on
	// [data-code] via overflow: scroll clip. In wrap mode lines
	// never overflow, so hide the track to remove the phantom gap.
	"[data-code] { scrollbar-width: none !important; }",
	"[data-code]::-webkit-scrollbar { height: 0 !important; }",
	DIFF_HEADER_CSS,
	SELECTION_OVERRIDE_CSS,
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
	if (!content) return null;
	const patch = Diff.createPatch(path, "", content, "", "");
	const parsed = parsePatchFiles(stripSvnIndexHeaders(patch));
	if (!parsed.length || !parsed[0].files.length) return null;
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
 * for a single file. Each edit becomes a separate
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

	const parsed = parsePatchFiles(stripSvnIndexHeaders(patches.join("")));
	if (!parsed.length || !parsed[0].files.length) return null;
	return parsed[0].files[0];
};

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
