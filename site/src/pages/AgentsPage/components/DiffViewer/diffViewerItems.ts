import type {
	CodeViewItem,
	DiffLineAnnotation,
	FileDiffMetadata,
} from "@pierre/diffs";
import { isMarkdownFileName } from "../../utils/markdownFile";
import {
	collectRenderedSegments,
	isWithinRenderBudget,
	RENDERED_MARKDOWN_ANNOTATION,
	renderedFileDiff,
} from "./renderedMarkdown";

export interface MarkdownPreviewState {
	isRendered: boolean;
	/** Set when the toggle must stay focusable but ignore activation. */
	disabledReason?: string;
}

export const PREVIEW_TOO_LARGE_REASON = "File too large to preview";
export const PREVIEW_BLOCKED_BY_COMMENT_REASON =
	"Finish the comment to preview";

/**
 * Derives preview state for every Markdown file that has something to
 * render. Eligibility is recomputed from the current parse, so a file that
 * grows past the budget or loses its hunks after being toggled is reported
 * as not rendered.
 */
export function resolveMarkdownPreviews(
	files: readonly FileDiffMetadata[],
	renderedFiles: ReadonlySet<string>,
	hasOpenComment: (fileName: string) => boolean,
): Map<string, MarkdownPreviewState> {
	const previews = new Map<string, MarkdownPreviewState>();
	for (const fileDiff of files) {
		if (!isMarkdownFileName(fileDiff.name)) {
			continue;
		}
		const rendered = collectRenderedSegments(fileDiff);
		if (rendered.segments.length === 0) {
			continue;
		}
		const withinBudget = isWithinRenderBudget(rendered);
		const isRendered = withinBudget && renderedFiles.has(fileDiff.name);
		let disabledReason: string | undefined;
		if (!withinBudget) {
			disabledReason = PREVIEW_TOO_LARGE_REASON;
		} else if (!isRendered && hasOpenComment(fileDiff.name)) {
			disabledReason = PREVIEW_BLOCKED_BY_COMMENT_REASON;
		}
		previews.set(fileDiff.name, { isRendered, disabledReason });
	}
	return previews;
}

/** Returns the rendered-file set with one file's preview state flipped. */
export function toggleRenderedFile(
	renderedFiles: ReadonlySet<string>,
	fileName: string,
): ReadonlySet<string> {
	const next = new Set(renderedFiles);
	if (!next.delete(fileName)) {
		next.add(fileName);
	}
	return next;
}

// CodeView's syncItemRecord skips reusing a record when item.version is
// unchanged, so the version must reflect annotation content rather than count.
// Moving the active comment box to another line in the same file keeps the
// count at 1 but must still re-render, so fold each annotation's side and line
// into the version.
export function annotationsVersion(
	annotations: readonly DiffLineAnnotation<string>[] | undefined,
): number {
	if (!annotations || annotations.length === 0) {
		return 0;
	}
	return annotations.reduce(
		(version, annotation) =>
			version * 31 +
			annotation.lineNumber * 2 +
			(annotation.side === "additions" ? 1 : 0),
		annotations.length,
	);
}

/**
 * Builds the CodeView item list. A previewed file becomes the empty-hunk
 * stand-in carrying a single file-level annotation; every other file keeps
 * its parsed diff and line annotations. The low bit of the version encodes
 * preview mode so a toggle always changes the version CodeView compares.
 */
export function buildDiffViewerItems(
	files: readonly FileDiffMetadata[],
	previews: ReadonlyMap<string, MarkdownPreviewState>,
	getLineAnnotations?: (fileName: string) => DiffLineAnnotation<string>[],
): CodeViewItem<string>[] {
	return files.map((fileDiff) => {
		const annotations = getLineAnnotations?.(fileDiff.name);
		if (previews.get(fileDiff.name)?.isRendered) {
			return {
				id: fileDiff.name,
				type: "diff",
				fileDiff: renderedFileDiff(fileDiff),
				annotations: [
					{
						side: fileDiff.type === "deleted" ? "deletions" : "additions",
						lineNumber: 0,
						metadata: RENDERED_MARKDOWN_ANNOTATION,
					},
				],
				version: annotationsVersion(annotations) * 2 + 1,
			};
		}
		return {
			id: fileDiff.name,
			type: "diff",
			fileDiff,
			annotations,
			version: annotationsVersion(annotations) * 2,
		};
	});
}
