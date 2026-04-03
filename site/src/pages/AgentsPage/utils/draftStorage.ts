/**
 * Utilities for persisting and restoring chat input drafts.
 *
 * Drafts are stored in localStorage. The current format stores the
 * serialized Lexical editor state JSON so that file-reference chips
 * survive navigation. Legacy drafts (plain-text strings) are detected
 * and handled transparently on read.
 */

export interface ParsedDraft {
	/** Plain text content for inputValueRef / send-button checks. */
	text: string;
	/**
	 * The raw Lexical serialized editor state JSON string, if the
	 * stored draft was in the structured format. `undefined` for
	 * legacy plain-text drafts.
	 */
	editorState: string | undefined;
}

/**
 * Read a draft from localStorage and determine whether it is a
 * Lexical editor state (JSON with a `root` key) or a legacy
 * plain-text string.
 */
export function parseStoredDraft(raw: string | null): ParsedDraft {
	if (!raw) {
		return { text: "", editorState: undefined };
	}
	try {
		const parsed = JSON.parse(raw);
		if (parsed?.root?.type === "root") {
			return { text: extractPlainText(parsed), editorState: raw };
		}
	} catch {
		// Not JSON — treat as legacy plain-text draft.
	}
	return { text: raw, editorState: undefined };
}

/**
 * Recursively walk a serialized Lexical node tree and extract the
 * concatenated plain text. Mirrors `$getRoot().getTextContent()`
 * without needing a live editor instance.
 */
function extractTextFromNode(node: {
	text?: string;
	children?: Array<Record<string, unknown>>;
	type?: string;
}): string {
	// Leaf text node.
	if (typeof node.text === "string") {
		return node.text;
	}
	// LineBreakNode serializes as { type: "linebreak" } with no
	// text or children. Lexical's getTextContent() returns "\n".
	if (node.type === "linebreak") {
		return "\n";
	}
	// FileReferenceNode and other non-text leaves contribute
	// nothing to plain text, matching getTextContent() behavior.
	if (!node.children) {
		return "";
	}
	const childTexts = node.children.map((child) =>
		extractTextFromNode(child as typeof node),
	);
	// Join root-level children (paragraphs) with double
	// newlines, matching Lexical's getTextContent() behavior.
	if (node.type === "root") {
		return childTexts.join("\n\n");
	}
	return childTexts.join("");
}

function extractPlainText(state: { root?: Record<string, unknown> }): string {
	if (!state.root) {
		return "";
	}
	return extractTextFromNode(
		state.root as Parameters<typeof extractTextFromNode>[0],
	);
}
