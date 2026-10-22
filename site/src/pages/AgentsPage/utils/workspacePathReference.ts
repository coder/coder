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

// Candidate code spans that start with an absolute or "~/" path and stand as
// whitespace-delimited tokens. splitWorkspacePathReferences keeps only those
// that formatWorkspacePathReference reproduces exactly.
const candidatePattern = /(?<=^|\s)(`+)( ?)((?:\/|~\/)[\s\S]*?)\2\1(?=\s|$)/g;

/** Splits text into plain text and the workspace path references in it. */
export const splitWorkspacePathReferences = (
	text: string,
): WorkspacePathTextSegment[] => {
	const segments: WorkspacePathTextSegment[] = [];
	let offset = 0;
	for (const match of text.matchAll(candidatePattern)) {
		const body = match[3] ?? "";
		const isDirectory = body.length > 1 && body.endsWith("/");
		const path = isDirectory ? body.slice(0, -1) : body;
		if (
			path === "/" ||
			path === "~" ||
			formatWorkspacePathReference(path, isDirectory) !== match[0]
		) {
			continue;
		}
		if (match.index > offset) {
			segments.push({ type: "text", text: text.slice(offset, match.index) });
		}
		segments.push({ type: "path", path, isDirectory });
		offset = match.index + match[0].length;
	}
	if (offset < text.length) {
		segments.push({ type: "text", text: text.slice(offset) });
	}
	return segments;
};
