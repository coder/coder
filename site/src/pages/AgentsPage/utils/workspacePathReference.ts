/** Lexical node type of a workspace file or folder chip in the composer. */
export const workspacePathNodeType = "workspace-path";

/** The JSON a workspace path chip serializes to in drafts and copies. */
type SerializedWorkspacePath = {
	type: typeof workspacePathNodeType;
	path: string;
	isDirectory?: unknown;
};

/**
 * Matches a serialized workspace path chip in JSON read from a draft or the
 * clipboard, where nothing guarantees the shape.
 */
export const isSerializedWorkspacePath = (node: {
	type?: unknown;
	path?: unknown;
	isDirectory?: unknown;
}): node is SerializedWorkspacePath =>
	node.type === workspacePathNodeType && typeof node.path === "string";

/**
 * The text a workspace file or folder chip sends: the path as Markdown inline
 * code, with a trailing "/" for a folder. The fence is one backtick longer
 * than any backtick run in the path, so every path stays one code span.
 */
export const formatWorkspacePathReference = (
	path: string,
	isDirectory: boolean,
): string => {
	const text = `${path}${isDirectory ? "/" : ""}`;
	let longestRun = 0;
	for (const run of text.match(/`+/g) ?? []) {
		longestRun = Math.max(longestRun, run.length);
	}
	const fence = "`".repeat(longestRun + 1);
	// Markdown strips one space from each side, so a path ending in a
	// backtick stays apart from the closing fence.
	const padding = text.endsWith("`") ? " " : "";
	return `${fence}${padding}${text}${padding}${fence}`;
};

type WorkspacePathTextSegment =
	| { type: "text"; text: string }
	| { type: "path"; path: string; isDirectory: boolean };

type CodeSpan = { start: number; end: number; fenceLength: number };

// Code spans as CommonMark delimits them: a backtick run closes at the next
// run of the same length, and a run that nothing closes is literal text. A
// precomputed next-run index visits each run once, so the scan is linear and
// a rejected span never reaches into the spans after it.
const findCodeSpans = (text: string): CodeSpan[] => {
	const runs = Array.from(text.matchAll(/`+/g), (match) => ({
		start: match.index,
		length: match[0].length,
	}));
	const nextRunOfLength: (number | undefined)[] = [];
	const laterRunByLength = new Map<number, number>();
	for (let index = runs.length - 1; index >= 0; index--) {
		nextRunOfLength[index] = laterRunByLength.get(runs[index].length);
		laterRunByLength.set(runs[index].length, index);
	}
	const spans: CodeSpan[] = [];
	let index = 0;
	while (index < runs.length) {
		const closing = nextRunOfLength[index];
		if (closing === undefined) {
			index++;
			continue;
		}
		spans.push({
			start: runs[index].start,
			end: runs[closing].start + runs[closing].length,
			fenceLength: runs[index].length,
		});
		index = closing + 1;
	}
	return spans;
};

const openingPunctuation = "([{\"'“‘";
const closingPunctuation = ".,;:!?)]}\"'”’";

// A reference follows whitespace or the start of the text, optionally after
// opening brackets or quotes.
const startsReference = (text: string, start: number): boolean => {
	let index = start;
	while (index > 0 && openingPunctuation.includes(text[index - 1])) {
		index--;
	}
	return index === 0 || /\s/.test(text[index - 1]);
};

// A reference precedes whitespace or the end of the text, optionally after
// sentence punctuation.
const endsReference = (text: string, end: number): boolean => {
	let index = end;
	while (index < text.length && closingPunctuation.includes(text[index])) {
		index++;
	}
	return index === text.length || /\s/.test(text[index]);
};

// The absolute or "~/" path of a code span that formatWorkspacePathReference
// reproduces exactly.
const getSpanReference = (
	text: string,
	span: CodeSpan,
): { path: string; isDirectory: boolean } | undefined => {
	if (!startsReference(text, span.start) || !endsReference(text, span.end)) {
		return undefined;
	}
	const content = text.slice(
		span.start + span.fenceLength,
		span.end - span.fenceLength,
	);
	const body =
		content.length > 1 && content.startsWith(" ") && content.endsWith(" ")
			? content.slice(1, -1)
			: content;
	const isDirectory = body.length > 1 && body.endsWith("/");
	const path = isDirectory ? body.slice(0, -1) : body;
	if (
		!/^~?\//.test(body) ||
		path === "/" ||
		path === "~" ||
		formatWorkspacePathReference(path, isDirectory) !==
			text.slice(span.start, span.end)
	) {
		return undefined;
	}
	return { path, isDirectory };
};

/** Splits text into plain text and the workspace path references in it. */
export const splitWorkspacePathReferences = (
	text: string,
): WorkspacePathTextSegment[] => {
	const segments: WorkspacePathTextSegment[] = [];
	let offset = 0;
	for (const span of findCodeSpans(text)) {
		const reference = getSpanReference(text, span);
		if (!reference) {
			continue;
		}
		if (span.start > offset) {
			segments.push({ type: "text", text: text.slice(offset, span.start) });
		}
		segments.push({ type: "path", ...reference });
		offset = span.end;
	}
	if (offset < text.length) {
		segments.push({ type: "text", text: text.slice(offset) });
	}
	return segments;
};
