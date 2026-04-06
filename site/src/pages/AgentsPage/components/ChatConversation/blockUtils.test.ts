import { describe, expect, it } from "vitest";
import { appendTextBlock, asNonEmptyString } from "./blockUtils";
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
		const result = appendTextBlock([], "thinking", "pondering");
		expect(result).toEqual([{ type: "thinking", text: "pondering" }]);
	});

	it("merges consecutive response blocks", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "aaa" }];
		const result = appendTextBlock(blocks, "response", "bbb");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({ type: "response", text: "aaabbb" });
	});

	it("merges consecutive thinking blocks", () => {
		const blocks: RenderBlock[] = [{ type: "thinking", text: "part1" }];
		const result = appendTextBlock(blocks, "thinking", "part2");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "part1part2",
		});
	});

	it("does not merge blocks of different types", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "hello" }];
		const result = appendTextBlock(blocks, "thinking", "hmm");
		expect(result).toHaveLength(2);
		expect(result[1]).toEqual({
			type: "thinking",
			text: "hmm",
		});
	});

	it("does not merge when last block is a tool block", () => {
		const blocks: RenderBlock[] = [{ type: "tool", id: "tool-1" }];
		const result = appendTextBlock(blocks, "response", "after tool");
		expect(result).toHaveLength(2);
		expect(result[1]).toEqual({ type: "response", text: "after tool" });
	});

	it("does not mutate the original blocks array", () => {
		const blocks: RenderBlock[] = [{ type: "response", text: "original" }];
		const result = appendTextBlock(blocks, "response", " added");
		expect(blocks).toHaveLength(1);
		expect((blocks[0] as { text: string }).text).toBe("original");
		expect(result).not.toBe(blocks);
	});
});
