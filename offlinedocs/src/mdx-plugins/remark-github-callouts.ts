// Convert GitHub-style alerts (`> [!NOTE]`) into Fumadocs `Callout` components.
// Runs at the mdast level; walks the tree manually rather than via
// `unist-util-visit`, which is not guaranteed to resolve under pnpm here.

import type { MdastNode } from "./mdast";

type GithubAlert = "NOTE" | "TIP" | "IMPORTANT" | "WARNING" | "CAUTION";

// GitHub alert -> Fumadocs Callout `type`.
const TYPE_MAP: Record<GithubAlert, string> = {
	NOTE: "info",
	TIP: "success",
	IMPORTANT: "idea",
	WARNING: "warn",
	CAUTION: "error",
};

const LABEL_MAP: Record<GithubAlert, string> = {
	NOTE: "Note",
	TIP: "Tip",
	IMPORTANT: "Important",
	WARNING: "Warning",
	CAUTION: "Caution",
};

// Strict on purpose: matches only GitHub's canonical marker (uppercase, no space after `!`).
const MARKER = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/;

function toCallout(blockquote: MdastNode): MdastNode | null {
	const firstParagraph = blockquote.children?.[0];
	if (
		!firstParagraph ||
		firstParagraph.type !== "paragraph" ||
		!firstParagraph.children
	) {
		return null;
	}

	const firstText = firstParagraph.children[0];
	if (
		!firstText ||
		firstText.type !== "text" ||
		typeof firstText.value !== "string"
	) {
		return null;
	}

	const match = MARKER.exec(firstText.value);
	if (!match) return null;
	const kind = match[1] as GithubAlert;

	firstText.value = firstText.value.slice(match[0].length);
	if (firstText.value === "") {
		firstParagraph.children.shift();
		// A hard line break sometimes follows the marker; drop it too.
		if (firstParagraph.children[0]?.type === "break") {
			firstParagraph.children.shift();
		}
	}
	if (firstParagraph.children.length === 0) {
		blockquote.children?.shift();
	}

	return {
		type: "mdxJsxFlowElement",
		name: "Callout",
		attributes: [
			{ type: "mdxJsxAttribute", name: "type", value: TYPE_MAP[kind] },
			{ type: "mdxJsxAttribute", name: "title", value: LABEL_MAP[kind] },
		],
		children: blockquote.children ?? [],
	};
}

function walk(node: MdastNode): void {
	if (!node.children) return;
	for (let i = 0; i < node.children.length; i++) {
		const child = node.children[i];
		if (child.type === "blockquote") {
			const callout = toCallout(child);
			if (callout) {
				node.children[i] = callout;
				walk(callout); // handle nested alerts inside the callout body
				continue;
			}
		}
		walk(child);
	}
}

export function remarkGithubCallouts() {
	return (tree: MdastNode): void => {
		walk(tree);
	};
}
