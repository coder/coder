/** Lexical node type of a workspace file or folder chip in the composer. */
export const workspacePathNodeType = "workspace-path";

/**
 * The text a workspace file or folder chip sends: the path in backticks,
 * with a trailing "/" for a folder.
 */
export const formatWorkspacePathReference = (
	path: string,
	isDirectory: boolean,
): string => `\`${path}${isDirectory ? "/" : ""}\``;

type WorkspacePathTextSegment =
	| { type: "text"; text: string }
	| { type: "path"; path: string; isDirectory: boolean };

// Only the exact form formatWorkspacePathReference produces for an absolute
// or "~/" path, as a whitespace-delimited token, so other inline code in the
// text stays as typed.
const workspacePathReferencePattern =
	/(?<=^|\s)`((?:\/|~\/)[^\s`]+?)(\/?)`(?=\s|$)/g;

/** Splits text into plain text and the workspace path references in it. */
export const splitWorkspacePathReferences = (
	text: string,
): WorkspacePathTextSegment[] => {
	const segments: WorkspacePathTextSegment[] = [];
	let offset = 0;
	for (const match of text.matchAll(workspacePathReferencePattern)) {
		if (match.index > offset) {
			segments.push({ type: "text", text: text.slice(offset, match.index) });
		}
		segments.push({
			type: "path",
			path: match[1] ?? "",
			isDirectory: match[2] === "/",
		});
		offset = match.index + match[0].length;
	}
	if (offset < text.length) {
		segments.push({ type: "text", text: text.slice(offset) });
	}
	return segments;
};
