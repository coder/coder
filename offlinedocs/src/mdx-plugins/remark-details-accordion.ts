// Convert raw `<details>`/`<summary>` HTML blocks into Fumadocs `Accordions`.
// In the plain-Markdown pipeline these arrive as separate raw `html` mdast
// nodes; this lifts the summary into a title and wraps the body in an Accordion.

import { isHtml, type MdastNode } from "./mdast";

// Recognized only when `<details>`/`</details>` are clean sibling raw-HTML nodes;
// trailing markup (`</details><br>`) or a nested `<details>` fall back to a native disclosure.
const DETAILS_OPEN = /^<details\b[^>]*>/i;
const DETAILS_CLOSE = /<\/details>\s*$/i;
const SUMMARY = /<summary\b[^>]*>([\s\S]*?)<\/summary>/i;

export function stripTags(html: string): string {
	return html
		.replace(/<[^>]+>/g, "")
		.replace(/\s+/g, " ")
		.trim();
}

type Built = { node: MdastNode; endIndex: number };

function buildAccordion(
	children: MdastNode[],
	startIndex: number,
): Built | null {
	const openNode = children[startIndex];
	if (!isHtml(openNode)) return null;

	let title: string | null = null;
	let contentStart = startIndex + 1;

	const inline = SUMMARY.exec(openNode.value);
	if (inline) {
		// `<details>` and `<summary>...</summary>` share one html node.
		title = stripTags(inline[1]);
	} else {
		// `<summary>...</summary>` is the next html node.
		const next = children[startIndex + 1];
		if (!isHtml(next)) return null;
		const match = SUMMARY.exec(next.value);
		if (!match) return null;
		title = stripTags(match[1]);
		contentStart = startIndex + 2;
	}

	let endIndex = -1;
	for (let j = contentStart; j < children.length; j++) {
		const node = children[j];
		// Nested <details> would need depth-tracking; bail so it falls back to native disclosure.
		if (isHtml(node) && DETAILS_OPEN.test(node.value.trim())) {
			return null;
		}
		if (isHtml(node) && DETAILS_CLOSE.test(node.value.trim())) {
			endIndex = j;
			break;
		}
	}
	if (endIndex === -1) return null;

	const content = children.slice(contentStart, endIndex);

	return {
		node: {
			type: "mdxJsxFlowElement",
			name: "Accordions",
			attributes: [{ type: "mdxJsxAttribute", name: "type", value: "single" }],
			children: [
				{
					type: "mdxJsxFlowElement",
					name: "Accordion",
					attributes: [
						{ type: "mdxJsxAttribute", name: "title", value: title ?? "" },
					],
					children: content,
				},
			],
		},
		endIndex,
	};
}

function transform(parent: MdastNode): void {
	if (!parent.children) return;
	for (const child of parent.children) transform(child);

	const out: MdastNode[] = [];
	for (let i = 0; i < parent.children.length; i++) {
		const node = parent.children[i];
		if (isHtml(node) && DETAILS_OPEN.test(node.value.trim())) {
			const built = buildAccordion(parent.children, i);
			if (built) {
				out.push(built.node);
				i = built.endIndex;
				continue;
			}
		}
		out.push(node);
	}
	parent.children = out;
}

export function remarkDetailsAccordion() {
	return (tree: MdastNode): void => {
		transform(tree);
	};
}
