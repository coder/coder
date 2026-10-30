import { describe, expect, it } from "vitest";
import {
	inlineNodeIndexProperty,
	inlineNodePlaceholder,
	rehypeInlineNodes,
	replaceInlineNodePlaceholders,
} from "./inlineNodes";

type Tree = Parameters<ReturnType<typeof rehypeInlineNodes>>[0];
type Children = Tree["children"];

const text = (value: string) => ({ type: "text" as const, value });
const element = (tagName: string, children: Children) => ({
	type: "element" as const,
	tagName,
	properties: {},
	children,
});
const placeholderSpan = (index: number) => ({
	type: "element" as const,
	tagName: "span",
	properties: { [inlineNodeIndexProperty]: index },
	children: [],
});

const transform = (children: Children) => {
	const tree: Tree = { children: structuredClone(children) };
	rehypeInlineNodes()(tree);
	return tree.children;
};

describe("rehypeInlineNodes", () => {
	it("turns placeholders into indexed spans, including in nested elements", () => {
		expect(
			transform([
				element("p", [
					text(`see ${inlineNodePlaceholder(0)} and `),
					element("strong", [text(`${inlineNodePlaceholder(1)}!`)]),
				]),
			]),
		).toEqual([
			element("p", [
				text("see "),
				placeholderSpan(0),
				text(" and "),
				element("strong", [placeholderSpan(1), text("!")]),
			]),
		]);
	});

	it("leaves placeholders inside fenced code as text", () => {
		const code = element("pre", [
			element("code", [text(`x ${inlineNodePlaceholder(0)}`)]),
		]);
		expect(transform([code])).toEqual([code]);
	});
});

describe("replaceInlineNodePlaceholders", () => {
	it("resolves placeholders to fallback text", () => {
		expect(
			replaceInlineNodePlaceholders(`open ${inlineNodePlaceholder(0)} now`, [
				"main.go:L1",
			]),
		).toBe("open main.go:L1 now");
	});
});
