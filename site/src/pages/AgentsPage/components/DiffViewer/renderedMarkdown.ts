import type { FileDiffMetadata } from "@pierre/diffs";
import type { UrlTransform } from "streamdown";

/**
 * Upper bound on the Markdown source rendered for one file. Streamdown parses
 * synchronously on the main thread and several GFM constructs (nested
 * brackets, wide tables) are super-linear, so larger files stay in raw mode.
 */
export const MAX_RENDERED_MARKDOWN_CHARS = 50_000;

/** Annotation metadata marking the file-level row that hosts the preview. */
export const RENDERED_MARKDOWN_ANNOTATION = "rendered-markdown";

type RenderedSide = "additions" | "deletions";

interface RenderedSegment {
	/** First line number of the segment in the rendered side of the file. */
	startLine: number;
	/** Last line number of the segment in the rendered side of the file. */
	endLine: number;
	text: string;
}

interface RenderedMarkdown {
	side: RenderedSide;
	/** One segment per hunk, in file order. Empty when nothing can render. */
	segments: RenderedSegment[];
	/** True when the segments make up the whole document. */
	isComplete: boolean;
	totalChars: number;
}

/**
 * Collects the text a preview can show for a diff. A unified diff only
 * carries hunks, so for a changed file each hunk becomes its own segment of
 * new-side text (context plus added lines). Deleted files use the old side
 * instead, which is the only side they have.
 */
export function collectRenderedSegments(
	fileDiff: FileDiffMetadata,
): RenderedMarkdown {
	const side: RenderedSide =
		fileDiff.type === "deleted" ? "deletions" : "additions";
	const sourceLines =
		side === "additions" ? fileDiff.additionLines : fileDiff.deletionLines;
	const segments: RenderedSegment[] = [];
	let totalChars = 0;

	for (const hunk of fileDiff.hunks) {
		const lines: string[] = [];
		for (const block of hunk.hunkContent) {
			const count =
				block.type === "context"
					? block.lines
					: side === "additions"
						? block.additions
						: block.deletions;
			const start =
				side === "additions"
					? block.additionLineIndex
					: block.deletionLineIndex;
			for (let i = 0; i < count; i++) {
				const line = sourceLines[start + i];
				if (line !== undefined) {
					// Parsed lines keep their terminator except for the last one
					// in the patch, so strip it before joining.
					lines.push(line.replace(/\r?\n$/, ""));
				}
			}
		}
		if (lines.length === 0) {
			continue;
		}
		const startLine =
			side === "additions" ? hunk.additionStart : hunk.deletionStart;
		const text = lines.join("\n");
		totalChars += text.length;
		segments.push({
			startLine,
			endLine: startLine + lines.length - 1,
			text,
		});
	}

	return {
		side,
		segments,
		isComplete: fileDiff.type === "new" || fileDiff.type === "deleted",
		totalChars,
	};
}

/** Whether the preview fits within the render budget. */
export function isWithinRenderBudget(rendered: RenderedMarkdown): boolean {
	return rendered.totalChars <= MAX_RENDERED_MARKDOWN_CHARS;
}

/**
 * Stand-in diff for a previewed file. CodeView renders a file-level
 * annotation only on a side the diff type allows, and a diff with no hunks
 * keeps its header and list position while showing no code rows. Deleted
 * files keep their type so the deletions side stays available; everything
 * else becomes `new` so split style does not reserve an empty deletions
 * column. The cache key must differ from the source diff's, since CodeView
 * compares diffs by cache key.
 */
export function renderedFileDiff(fileDiff: FileDiffMetadata): FileDiffMetadata {
	return {
		...fileDiff,
		type: fileDiff.type === "deleted" ? "deleted" : "new",
		hunks: [],
		additionLines: [],
		deletionLines: [],
		splitLineCount: 0,
		unifiedLineCount: 0,
		cacheKey: `${fileDiff.cacheKey ?? fileDiff.name}:rendered`,
	};
}

const LINK_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);
const IMAGE_PROTOCOLS = new Set(["http:", "https:"]);

const normalizeHostname = (hostname: string) => hostname.replace(/\.$/, "");

/**
 * URL policy for Markdown that came from a repository rather than the
 * deployment. Relative paths have no base to resolve against here, so they
 * are dropped instead of resolving to the Coder origin. Image sources on the
 * deployment's host are dropped on any scheme or port: path-based workspace
 * apps and the API share that host, so a same-host request is not harmless.
 * Any attribute other than `href` and `src` is dropped.
 */
export const renderedMarkdownUrlTransform: UrlTransform = (url, key) => {
	let parsed: URL;
	try {
		parsed = new URL(url);
	} catch {
		return null;
	}
	switch (key) {
		case "src":
			if (!IMAGE_PROTOCOLS.has(parsed.protocol)) {
				return null;
			}
			return normalizeHostname(parsed.hostname) ===
				normalizeHostname(location.hostname)
				? null
				: url;
		case "href":
			return LINK_PROTOCOLS.has(parsed.protocol) ? url : null;
		default:
			return null;
	}
};
