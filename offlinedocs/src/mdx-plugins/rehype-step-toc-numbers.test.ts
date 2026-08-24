import assert from "node:assert/strict";
import { test } from "node:test";

import { rehypeStepTocNumbers, type HastNode } from "./rehype-step-toc-numbers";

const element = (
	tagName: string,
	properties: Record<string, unknown>,
	children: HastNode[] = [],
): HastNode => ({ type: "element", tagName, properties, children });

function run(children: HastNode[]): HastNode {
	const tree: HastNode = { type: "root", children };
	rehypeStepTocNumbers()(tree);
	return tree;
}

const propsOf = (node: HastNode): Record<string, unknown> =>
	node.properties ?? {};

test("a stringified dataFdStep is restored to a numeric data-fd-step", () => {
	const tree = run([element("h2", { dataFdStep: "3" })]);
	const props = propsOf(tree.children![0]);
	assert.equal(props["data-fd-step"], 3);
	assert.equal(typeof props["data-fd-step"], "number");
	assert.equal("dataFdStep" in props, false);
});

test("a numeric data-fd-step is preserved", () => {
	const tree = run([element("h3", { "data-fd-step": 2 })]);
	assert.equal(propsOf(tree.children![0])["data-fd-step"], 2);
});

test("a non-numeric step value is left untouched", () => {
	const tree = run([element("h2", { dataFdStep: "abc" })]);
	const props = propsOf(tree.children![0]);
	assert.equal(props.dataFdStep, "abc");
	assert.equal(props["data-fd-step"], undefined);
});

test("non-heading elements are ignored", () => {
	const tree = run([element("div", { dataFdStep: "3" })]);
	const props = propsOf(tree.children![0]);
	assert.equal(props.dataFdStep, "3");
	assert.equal(props["data-fd-step"], undefined);
});

test("nested headings are processed", () => {
	const tree = run([element("div", {}, [element("h4", { dataFdStep: "5" })])]);
	const props = propsOf(tree.children![0].children![0]);
	assert.equal(props["data-fd-step"], 5);
	assert.equal("dataFdStep" in props, false);
});
