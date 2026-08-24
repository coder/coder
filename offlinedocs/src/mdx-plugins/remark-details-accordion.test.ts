import assert from "node:assert/strict";
import { test } from "node:test";

import type { MdastNode } from "./mdast";
import { remarkDetailsAccordion, stripTags } from "./remark-details-accordion";

test("stripTags removes inline tags and collapses whitespace", () => {
	assert.equal(stripTags("<b>Hi</b>   there"), "Hi there");
	assert.equal(stripTags("Show <code>config.yaml</code>"), "Show config.yaml");
	assert.equal(stripTags("  spaced  "), "spaced");
	assert.equal(stripTags(""), "");
});

const html = (value: string): MdastNode => ({ type: "html", value });
const para = (value: string): MdastNode => ({
	type: "paragraph",
	children: [{ type: "text", value }],
});
const kids = (n: MdastNode): MdastNode[] => (n.children ?? []) as MdastNode[];
const attrs = (n: MdastNode): MdastNode[] =>
	(n.attributes as MdastNode[]) ?? [];
const listOf = (children: MdastNode[]): MdastNode => ({
	type: "list",
	children: [{ type: "listItem", children }],
});

function run(children: MdastNode[]): MdastNode {
	const tree: MdastNode = { type: "root", children };
	remarkDetailsAccordion()(tree);
	return tree;
}

test("an inline <details><summary> becomes a single-item Accordions", () => {
	const tree = run([
		html("<details><summary>Advanced</summary>"),
		para("body"),
		html("</details>"),
	]);
	assert.equal(kids(tree).length, 1);
	const accordions = kids(tree)[0];
	assert.equal(accordions.type, "mdxJsxFlowElement");
	assert.equal(accordions.name, "Accordions");
	assert.deepEqual(
		attrs(accordions).map((a) => [a.name, a.value]),
		[["type", "single"]],
	);
	const accordion = kids(accordions)[0];
	assert.equal(accordion.name, "Accordion");
	assert.deepEqual(
		attrs(accordion).map((a) => [a.name, a.value]),
		[["title", "Advanced"]],
	);
	assert.deepEqual(kids(accordion), [para("body")]);
});

test("a separate <summary> node is used as the title", () => {
	const tree = run([
		html("<details>"),
		html("<summary>More info</summary>"),
		para("body"),
		html("</details>"),
	]);
	const accordion = kids(kids(tree)[0])[0];
	assert.equal(attrs(accordion)[0].value, "More info");
	assert.deepEqual(kids(accordion), [para("body")]);
});

test("inline tags in the summary are stripped from the title", () => {
	const tree = run([
		html("<details><summary>Show <code>config.yaml</code></summary>"),
		para("body"),
		html("</details>"),
	]);
	const accordion = kids(kids(tree)[0])[0];
	assert.equal(attrs(accordion)[0].value, "Show config.yaml");
});

test("an unclosed <details> block is left untouched", () => {
	const tree = run([html("<details><summary>X</summary>"), para("body")]);
	assert.equal(kids(tree).length, 2);
	assert.equal(kids(tree)[0].type, "html");
});

test("a </details><br> close is left as native disclosure (not converted)", () => {
	const tree = run([
		html("<details><summary>X</summary>"),
		para("body"),
		html("</details><br>"),
	]);
	assert.equal(kids(tree).length, 3);
	assert.equal(kids(tree)[0].type, "html");
	assert.equal(kids(tree)[1].type, "paragraph");
	assert.equal(kids(tree)[2].type, "html");
});

test("a <details> inside a list item converts when delimiters are clean", () => {
	const tree = run([
		listOf([
			html("<details><summary>Nested</summary>"),
			para("body"),
			html("</details>"),
		]),
	]);
	const list = kids(tree)[0];
	assert.equal(list.type, "list");
	const item = kids(list)[0];
	assert.equal(item.type, "listItem");
	assert.equal(kids(item).length, 1);
	const accordions = kids(item)[0];
	assert.equal(accordions.type, "mdxJsxFlowElement");
	assert.equal(accordions.name, "Accordions");
	const accordion = kids(accordions)[0];
	assert.equal(accordion.name, "Accordion");
	assert.deepEqual(
		attrs(accordion).map((a) => [a.name, a.value]),
		[["title", "Nested"]],
	);
	assert.deepEqual(kids(accordion), [para("body")]);
});

test("a nested <details> falls back to native disclosure (outer not converted)", () => {
	const tree = run([
		html("<details><summary>Outer</summary>"),
		html("<details><summary>Inner</summary>"),
		para("inner body"),
		html("</details>"),
		para("outer body"),
		html("</details>"),
	]);
	assert.equal(kids(tree)[0].type, "html");
	// Inner has a clean close so it converts independently.
	const innerAccordions = kids(tree)[1];
	assert.equal(innerAccordions.type, "mdxJsxFlowElement");
	assert.equal(innerAccordions.name, "Accordions");
	assert.equal(kids(tree)[2].type, "paragraph");
	assert.equal(kids(tree)[3].type, "html");
	assert.equal(kids(tree).length, 4);
});
