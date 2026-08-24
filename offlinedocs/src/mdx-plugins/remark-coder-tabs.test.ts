import assert from "node:assert/strict";
import { test } from "node:test";

import type { MdastNode } from "./mdast";
import {
	canonicalOS,
	divDelta,
	groupIdFor,
	remarkCoderTabs,
	splitOSLabel,
} from "./remark-coder-tabs";

test("canonicalOS normalizes known spellings and rejects the rest", () => {
	assert.equal(canonicalOS("linux"), "Linux");
	assert.equal(canonicalOS("  macOS  "), "macOS");
	assert.equal(canonicalOS("OSX"), "macOS");
	assert.equal(canonicalOS("darwin"), "macOS");
	assert.equal(canonicalOS("win"), "Windows");
	assert.equal(canonicalOS("Debian"), null);
});

test("splitOSLabel splits OS-only labels (and dedupes), else returns null", () => {
	assert.deepEqual(splitOSLabel("Linux/macOS"), ["Linux", "macOS"]);
	assert.deepEqual(splitOSLabel("macOS, Windows"), ["macOS", "Windows"]);
	assert.deepEqual(splitOSLabel("Linux & Windows"), ["Linux", "Windows"]);
	assert.deepEqual(splitOSLabel("macOS and Linux"), ["macOS", "Linux"]);
	// mac, osx and darwin all canonicalize to macOS, so the result is deduped.
	assert.deepEqual(splitOSLabel("mac/osx/darwin"), ["macOS"]);
	assert.equal(splitOSLabel("Debian, Ubuntu"), null);
	assert.equal(splitOSLabel("UI"), null);
	assert.equal(splitOSLabel(""), null);
});

test("groupIdFor is order-independent and slugified", () => {
	assert.equal(groupIdFor(["UI", "CLI"]), "cli-ui");
	assert.equal(groupIdFor(["CLI", "UI"]), "cli-ui");
	assert.equal(groupIdFor(["Docker", "Kubernetes"]), "docker-kubernetes");
});

test("divDelta counts net <div> nesting from a raw html node", () => {
	assert.equal(divDelta('<div class="tabs">'), 1);
	assert.equal(divDelta("</div>"), -1);
	assert.equal(divDelta("<div><div>"), 2);
	assert.equal(divDelta("</div></div>"), -2);
	assert.equal(divDelta("<div></div>"), 0);
	assert.equal(divDelta("plain text"), 0);
});

const html = (value: string): MdastNode => ({ type: "html", value });
const heading = (depth: number, value: string): MdastNode => ({
	type: "heading",
	depth,
	children: [{ type: "text", value }],
});
const para = (value: string): MdastNode => ({
	type: "paragraph",
	children: [{ type: "text", value }],
});
const kids = (n: MdastNode): MdastNode[] => (n.children ?? []) as MdastNode[];
const attrs = (n: MdastNode): MdastNode[] =>
	(n.attributes as MdastNode[]) ?? [];

function run(children: MdastNode[]): MdastNode {
	const tree: MdastNode = { type: "root", children };
	remarkCoderTabs()(tree);
	return tree;
}

test("a non-OS tab set becomes <Tabs> with items, groupId and persist", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "UI"),
		para("ui"),
		heading(3, "CLI"),
		para("cli"),
		html("</div>"),
	]);
	assert.equal(kids(tree).length, 1);
	const tabs = kids(tree)[0];
	assert.equal(tabs.type, "mdxJsxFlowElement");
	assert.equal(tabs.name, "Tabs");
	assert.deepEqual(
		attrs(tabs).map((a) => a.name),
		["items", "groupId", "persist"],
	);
	assert.deepEqual(
		kids(tabs).map((tab) => attrs(tab)[0].value),
		["UI", "CLI"],
	);
});

test("an all-OS tab set becomes <OSTab> with only an items attribute", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "Linux"),
		para("linux"),
		heading(3, "macOS"),
		para("mac"),
		heading(3, "Windows"),
		para("win"),
		html("</div>"),
	]);
	const set = kids(tree)[0];
	assert.equal(set.name, "OSTab");
	assert.deepEqual(
		attrs(set).map((a) => a.name),
		["items"],
	);
	assert.equal(kids(set).length, 3);
});

test("a bundled OS heading expands into one tab per OS", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "Linux/macOS"),
		para("shared"),
		html("</div>"),
	]);
	const set = kids(tree)[0];
	assert.equal(set.name, "OSTab");
	assert.deepEqual(
		kids(set).map((tab) => attrs(tab)[0].value),
		["Linux", "macOS"],
	);
});

test("solo OS alias headings emit under canonical names", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "OSX"),
		para("mac"),
		heading(3, "Windows"),
		para("win"),
		html("</div>"),
	]);
	const set = kids(tree)[0];
	assert.equal(set.name, "OSTab");
	const items = (
		attrs(set)[0] as any
	).value.data.estree.body[0].expression.elements.map(
		(e: { value: string }) => e.value,
	);
	assert.deepEqual(items, ["macOS", "Windows"]);
	assert.deepEqual(
		kids(set).map((tab) => attrs(tab)[0].value),
		["macOS", "Windows"],
	);
});

test("structuredClone isolates duplicated content across expanded OS tabs", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "Linux/macOS"),
		html('<div class="tabs">'),
		heading(4, "Sub1"),
		para("sub1"),
		heading(4, "Sub2"),
		para("sub2"),
		html("</div>"),
		html("</div>"),
	]);
	const set = kids(tree)[0];
	assert.equal(set.name, "OSTab");
	const linuxTab = kids(set)[0];
	const macTab = kids(set)[1];
	assert.equal(attrs(linuxTab)[0].value, "Linux");
	assert.equal(attrs(macTab)[0].value, "macOS");
	assert.notStrictEqual(linuxTab.children, macTab.children);
	const lenBefore = (macTab.children as MdastNode[]).length;
	const sentinel: MdastNode = { type: "text", value: "SENTINEL" };
	(linuxTab.children as MdastNode[]).push(sentinel);
	assert.equal((macTab.children as MdastNode[]).length, lenBefore);
});

test("an unclosed tabs block is left untouched", () => {
	const tree = run([html('<div class="tabs">'), heading(3, "UI"), para("ui")]);
	assert.equal(kids(tree)[0].type, "html");
	assert.equal(kids(tree).length, 3);
});

test("nested tabs are converted before the outer split", () => {
	const tree = run([
		html('<div class="tabs">'),
		heading(3, "Linux"),
		html('<div class="tabs">'),
		heading(4, "Debian"),
		para("deb"),
		heading(4, "Ubuntu"),
		para("ubuntu"),
		html("</div>"),
		heading(3, "Windows"),
		para("win"),
		html("</div>"),
	]);
	const outer = kids(tree)[0];
	assert.equal(outer.name, "OSTab");
	assert.deepEqual(
		kids(outer).map((tab) => attrs(tab)[0].value),
		["Linux", "Windows"],
	);
	const linuxTab = kids(outer)[0];
	const nested = kids(linuxTab).find((n) => n.type === "mdxJsxFlowElement");
	assert.ok(nested, "expected a nested tabs element inside the Linux tab");
	assert.equal(nested?.name, "Tabs");
	assert.deepEqual(
		kids(nested as MdastNode).map((tab) => attrs(tab)[0].value),
		["Debian", "Ubuntu"],
	);
});
