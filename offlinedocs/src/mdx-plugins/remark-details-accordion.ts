// Convert raw `<details>`/`<summary>` HTML blocks into Fumadocs `Accordions`.
//
// In the plain-Markdown (.md) pipeline, `<details>`, `<summary>...</summary>`,
// and `</details>` arrive as separate raw `html` mdast nodes with the body
// content as normal Markdown nodes in between (the same shape the corpus uses
// for `<div class="tabs">`). This plugin finds that run, lifts the summary text
// into an accordion title, and wraps the body in a `Callout`-style
// `mdxJsxFlowElement` (`Accordions` > `Accordion`) so it renders through the
// component map. Runs at the mdast level, so no per-file rewrites are needed.
//
// Each `<details>` becomes its own single-item `<Accordions type="single">`,
// which Fumadocs renders collapsible: it opens and closes exactly like the
// native element. The summary is flattened to text (any inline tags such as
// `<code>` are dropped) because the title is passed as a string attribute.

import { isHtml, type MdastNode } from "./mdast";

// A `<details>` block is recognized only when its opener and closer arrive as
// sibling raw-HTML nodes: DETAILS_OPEN must start the opening node and
// DETAILS_CLOSE must end the closing node. `transform` recurses into every
// parent first, so a `<details>` nested inside a list item is still converted,
// as long as those two delimiters are clean.
//
// Known limitation: a closing node that carries trailing markup on the same
// line, e.g. `</details><br>`, does not match DETAILS_CLOSE, so the block is
// left as a native `<details>` disclosure instead of an Accordion. The fallback
// still renders (the inner Markdown, including callouts, converts); it is just
// not Accordion-styled. Author `<details>` and `</details>` each alone on their
// own line to get the Accordion.
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
		// `<summary>...</summary>` is the next html node (blank line after
		// `<details>`).
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
