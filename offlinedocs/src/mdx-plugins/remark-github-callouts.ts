// Convert GitHub-style alerts into Fumadocs `Callout` components.
//
// GitHub alert syntax (https://github.com/orgs/community/discussions/16925):
//
//   > [!NOTE]
//   > Body text.
//
// Fumadocs does not convert these out of the box: its `remarkAdmonition`
// handles only Docusaurus `:::` syntax (and is deprecated), and `remark-gfm`
// leaves `[!NOTE]` as literal text inside a blockquote. This plugin rewrites
// the blockquote into a `Callout` MDX element node so it renders through the
// Fumadocs component map. It runs at the mdast level, so it works on plain
// `.md` files with no per-file rewrites.
//
// The plugin is written without `unist-util-visit` on purpose: under pnpm's
// strict node_modules it is not guaranteed to resolve from this file, and the
// walk we need is trivial.

import type { MdastNode } from "./mdast";

type GithubAlert = "NOTE" | "TIP" | "IMPORTANT" | "WARNING" | "CAUTION";

// GitHub alert -> Fumadocs Callout `type`. The mapping follows GitHub's own
// color semantics (note=blue, tip=green, important=purple, warning=amber,
// caution=red).
const TYPE_MAP: Record<GithubAlert, string> = {
	NOTE: "info",
	TIP: "success",
	IMPORTANT: "idea",
	WARNING: "warn",
	CAUTION: "error",
};

// Preserve the author's intent as the callout title, matching GitHub's label.
const LABEL_MAP: Record<GithubAlert, string> = {
	NOTE: "Note",
	TIP: "Tip",
	IMPORTANT: "Important",
	WARNING: "Warning",
	CAUTION: "Caution",
};

// Strict on purpose: this matches only GitHub's canonical marker (uppercase
// type, no space after `!`). Non-canonical markers (`[! WARNING]`, `[!Note]`)
// are normalized at the source instead, so the matcher stays aligned with
// GitHub's own rendering.
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

	// Strip the `[!TYPE]` marker (and its trailing newline) from the body.
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
