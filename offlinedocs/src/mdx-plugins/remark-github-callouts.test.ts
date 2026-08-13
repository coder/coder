import assert from "node:assert/strict";
import { test } from "node:test";

import { remarkGithubCallouts } from "./remark-github-callouts";

// The plugin's MdastNode type is internal, so mirror the minimal shape here.
type Node = {
	type: string;
	children?: Node[];
	value?: string;
	[key: string]: unknown;
};

const text = (value: string): Node => ({ type: "text", value });
const para = (children: Node[]): Node => ({ type: "paragraph", children });
const quote = (children: Node[]): Node => ({ type: "blockquote", children });
const kids = (n: Node): Node[] => (n.children ?? []) as Node[];
const attrs = (n: Node): Node[] => (n.attributes as Node[]) ?? [];

function run(children: Node[]): Node {
	const tree: Node = { type: "root", children };
	remarkGithubCallouts()(tree);
	return tree;
}

test("a [!NOTE] blockquote becomes a Callout with the marker stripped", () => {
	const tree = run([quote([para([text("[!NOTE] Body text.")])])]);
	const callout = kids(tree)[0];
	assert.equal(callout.type, "mdxJsxFlowElement");
	assert.equal(callout.name, "Callout");
	assert.deepEqual(
		attrs(callout).map((a) => [a.name, a.value]),
		[
			["type", "info"],
			["title", "Note"],
		],
	);
	// The [!NOTE] marker is removed but the body text stays.
	assert.equal(kids(kids(callout)[0])[0].value, "Body text.");
});

test("each GitHub alert maps to its Callout type and title", () => {
	const cases: Array<[string, string, string]> = [
		["NOTE", "info", "Note"],
		["TIP", "success", "Tip"],
		["IMPORTANT", "idea", "Important"],
		["WARNING", "warn", "Warning"],
		["CAUTION", "error", "Caution"],
	];
	for (const [marker, type, title] of cases) {
		const tree = run([quote([para([text(`[!${marker}] x`)])])]);
		const callout = kids(tree)[0];
		assert.equal(callout.name, "Callout");
		assert.deepEqual(
			attrs(callout).map((a) => a.value),
			[type, title],
		);
	}
});

test("a plain blockquote is left untouched", () => {
	const tree = run([quote([para([text("Just a quote.")])])]);
	assert.equal(kids(tree)[0].type, "blockquote");
});

test("a marker on its own line drops the trailing break", () => {
	const tree = run([
		quote([para([text("[!WARNING]"), { type: "break" }, text("Body")])]),
	]);
	const callout = kids(tree)[0];
	assert.equal(callout.name, "Callout");
	const bodyPara = kids(callout)[0];
	// The now-empty marker text and the following hard break are both removed.
	assert.deepEqual(kids(bodyPara), [text("Body")]);
});

test("nested alerts inside a callout body are converted", () => {
	const tree = run([
		quote([
			para([text("[!NOTE] Outer")]),
			quote([para([text("[!TIP] Inner")])]),
		]),
	]);
	const outer = kids(tree)[0];
	assert.equal(outer.name, "Callout");
	const nested = kids(outer)[1];
	assert.equal(nested.name, "Callout");
	assert.equal(attrs(nested)[0].value, "success");
});
