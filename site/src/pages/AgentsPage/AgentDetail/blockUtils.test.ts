import { describe, expect, it } from "vitest";
import {
	appendTextBlock,
	asNonEmptyString,
	mergeThinkingTitles,
} from "./blockUtils";
import type { RenderBlock } from "./types";

// ---------------------------------------------------------------------------
// asNonEmptyString
// ---------------------------------------------------------------------------

describe("asNonEmptyString", () => {
	it("returns the string when it is non-empty", () => {
		expect(asNonEmptyString("hello")).toBe("hello");
	});

	it("returns trimmed string when value has whitespace", () => {
		expect(asNonEmptyString("  hello  ")).toBe("hello");
	});

	it("returns undefined for an empty string", () => {
		expect(asNonEmptyString("")).toBeUndefined();
	});

	it("returns undefined for a whitespace-only string", () => {
		expect(asNonEmptyString("   ")).toBeUndefined();
	});

	it("returns undefined for non-string values", () => {
		expect(asNonEmptyString(undefined)).toBeUndefined();
		expect(asNonEmptyString(null)).toBeUndefined();
		expect(asNonEmptyString(42)).toBeUndefined();
		expect(asNonEmptyString(true)).toBeUndefined();
		expect(asNonEmptyString({})).toBeUndefined();
	});
});

// ---------------------------------------------------------------------------
// mergeThinkingTitles
// ---------------------------------------------------------------------------

describe("mergeThinkingTitles", () => {
	it("merges when both titles are undefined", () => {
		expect(mergeThinkingTitles(undefined, undefined)).toEqual({
			shouldMerge: true,
			title: undefined,
		});
	});

	it("merges and picks nextTitle when current is undefined", () => {
		expect(mergeThinkingTitles(undefined, "Thinking")).toEqual({
			shouldMerge: true,
			title: "Thinking",
		});
	});

	it("merges and keeps currentTitle when next is undefined", () => {
		expect(mergeThinkingTitles("Thinking", undefined)).toEqual({
			shouldMerge: true,
			title: "Thinking",
		});
	});

	it("merges when titles are identical", () => {
		expect(mergeThinkingTitles("Thinking", "Thinking")).toEqual({
			shouldMerge: true,
			title: "Thinking",
		});
	});

	it("merges and uses nextTitle when it extends currentTitle", () => {
		expect(mergeThinkingTitles("Think", "Thinking deeply")).toEqual({
			shouldMerge: true,
			title: "Thinking deeply",
		});
	});

	it("merges and keeps currentTitle when it extends nextTitle", () => {
		expect(mergeThinkingTitles("Thinking deeply", "Think")).toEqual({
			shouldMerge: true,
			title: "Thinking deeply",
		});
	});

	it("does not merge when titles are completely different", () => {
		expect(mergeThinkingTitles("Analyzing", "Planning")).toEqual({
			shouldMerge: false,
			title: "Planning",
		});
	});
});

// ---------------------------------------------------------------------------
// appendTextBlock
// ---------------------------------------------------------------------------

describe("appendTextBlock", () => {
	it("returns the same blocks when text is empty or whitespace", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "hello" }];
		expect(appendTextBlock(blocks, "response", "")).toBe(blocks);
		expect(appendTextBlock(blocks, "response", "   ")).toBe(blocks);
		expect(appendTextBlock(blocks, "thinking", "\n\t")).toBe(blocks);
	});

	it("appends a new response block to an empty list", () => {
		const result = appendTextBlock([], "response", "hello");
		expect(result).toEqual([{ type: "response", text: "hello" }]);
	});

	it("appends a new thinking block to an empty list", () => {
		const result = appendTextBlock([], "thinking", "pondering", "Deep thought");
		expect(result).toEqual([
			{ type: "thinking", text: "pondering", title: "Deep thought" },
		]);
	});

	it("merges consecutive response blocks", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "aaa" }];
		const result = appendTextBlock(blocks, "response", "bbb");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({ type: "response", text: "aaabbb" });
	});

	it("merges consecutive thinking blocks with compatible titles", () => {
		const blocks: RenderBlock[] = [
			{ type: "thinking", text: "part1", title: "Reasoning" },
		];
		const result = appendTextBlock(blocks, "thinking", "part2", "Reasoning");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "part1part2",
			title: "Reasoning",
		});
	});

	it("does not merge thinking blocks with incompatible titles", () => {
		const blocks: RenderBlock[] = [
			{ type: "thinking", text: "part1", title: "Analyzing" },
		];
		const result = appendTextBlock(blocks, "thinking", "part2", "Planning");
		expect(result).toHaveLength(2);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "part1",
			title: "Analyzing",
		});
		expect(result[1]).toEqual({
			type: "thinking",
			text: "part2",
			title: "Planning",
		});
	});

	it("does not merge blocks of different types", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "hello" }];
		const result = appendTextBlock(blocks, "thinking", "hmm");
		expect(result).toHaveLength(2);
		expect(result[1]).toEqual({
			type: "thinking",
			text: "hmm",
			title: undefined,
		});
	});

	it("does not merge when last block is a tool block", () => {
		const blocks: RenderBlock[] = [{ type: "tool", id: "tool-1" }];
		const result = appendTextBlock(blocks, "response", "after tool");
		expect(result).toHaveLength(2);
		expect(result[1]).toEqual({ type: "response", text: "after tool" });
	});

	it("uses the custom joinText function when merging", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "line1" }];
		const join = (a: string, b: string) => `${a}\n${b}`;
		const result = appendTextBlock(
			blocks,
			"response",
			"line2",
			undefined,
			join,
		);
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({ type: "response", text: "line1\nline2" });
	});

	it("does not mutate the original blocks array", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "original" }];
		const result = appendTextBlock(blocks, "response", " added");
		expect(blocks).toHaveLength(1);
		expect((blocks[0] as { text: string }).text).toBe("original");
		expect(result).not.toBe(blocks);
	});

	it("merges thinking block when nextTitle extends currentTitle", () => {
		const blocks: RenderBlock[] = [
			{ type: "thinking", text: "a", title: "Think" },
		];
		const result = appendTextBlock(blocks, "thinking", "b", "Thinking deeply");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "ab",
			title: "Thinking deeply",
		});
	});

	it("merges thinking blocks when both have no title", () => {
		const blocks: RenderBlock[] = [{ type: "thinking", text: "a" }];
		const result = appendTextBlock(blocks, "thinking", "b");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "ab",
			title: undefined,
		});
	});
});
