import type { Nodes, Root } from "mdast";
import remarkParse from "remark-parse";
import { unified } from "unified";
import { describe, expect, it } from "vitest";
import { chatRemarkPlugins } from "./chatMarkdown";

const processor = unified().use(remarkParse).use(chatRemarkPlugins);

// The plugins transform the tree in place.
const parse = (markdown: string): Root => {
	const tree = processor.parse(markdown);
	processor.runSync(tree);
	return tree;
};

const flatten = (node: Nodes): Nodes[] => {
	if (!("children" in node)) {
		return [node];
	}
	const children: readonly Nodes[] = node.children;
	return [node, ...children.flatMap(flatten)];
};

// The item numbers set for rendering, or null where the browser numbers it.
const getListItemValues = (markdown: string) =>
	flatten(parse(markdown)).flatMap((node) =>
		node.type === "listItem" ? [node.data?.hProperties?.value ?? null] : [],
	);

const hasStrikethrough = (markdown: string) =>
	flatten(parse(markdown)).some((node) => node.type === "delete");

describe("chatRemarkPlugins", () => {
	describe("ordered list numbers", () => {
		it.each([
			[
				"a list that skips a step",
				"1. One\n3. Three\n4. Four",
				["1", "3", "4"],
			],
			[
				"a list that starts after one and skips",
				"3. Three\n5. Five",
				["3", "5"],
			],
			["parenthesis markers", "1) One\n4) Four", ["1", "4"]],
			["a loose list", "1. One\n\n3. Three\n\n7. Seven", ["1", "3", "7"]],
			["a list in a block quote", "> 1. One\n> 3. Three", ["1", "3"]],
		])("keeps the written numbers of %s", (_, markdown, values) => {
			expect(getListItemValues(markdown)).toEqual(values);
		});

		it.each([
			["repeats its first number", "1. One\n1. Two\n1. Three"],
			["counts up from its start", "3. Three\n4. Four\n5. Five"],
		])("leaves numbering to the list when it %s", (_, markdown) => {
			expect(getListItemValues(markdown)).toEqual([null, null, null]);
		});

		it("numbers a nested list on its own", () => {
			expect(
				getListItemValues(
					"1. Outer\n   1. Inner\n   3. Inner skipped\n2. Outer next",
				),
			).toEqual([null, "1", "3", null]);
		});
	});

	describe("strikethrough", () => {
		it("leaves single tildes as text", () => {
			expect(
				hasStrikethrough("Each step takes ~5 min, so 10~15 min total."),
			).toBe(false);
		});

		it("strikes text between double tildes", () => {
			expect(hasStrikethrough("Use ~~the old flag~~ the new one.")).toBe(true);
		});
	});
});
