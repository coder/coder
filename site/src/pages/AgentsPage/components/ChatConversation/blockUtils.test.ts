import { describe, expect, it } from "vitest";
import {
	appendTextBlock,
	asNonEmptyString,
	toTimelineBlocks,
} from "./blockUtils";
import type { MergedTool, RenderBlock } from "./types";

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

describe("toTimelineBlocks", () => {
	const tool = (id: string, name = "read_file"): MergedTool => ({
		id,
		name,
		isError: false,
		status: "completed",
	});
	const executeTool = (id: string): MergedTool => ({
		...tool(id, "execute"),
		args: { command: "ls" },
	});
	const suppressedWaitTool: MergedTool = {
		id: "wait-1",
		name: "wait_agent",
		isError: false,
		status: "running",
	};
	// A truncated args fragment, as it arrives mid-stream.
	const streamingExecuteTool: MergedTool = {
		id: "execute-pending",
		name: "execute",
		args: '{"command": "git ch',
		isError: false,
		status: "running",
	};
	const commandlessExecuteWithError: MergedTool = {
		id: "execute-failed",
		name: "execute",
		args: {},
		result: { error: "command is required" },
		isError: true,
		status: "error",
	};
	const tools = [
		tool("read-1"),
		tool("read-2"),
		executeTool("execute-1"),
		streamingExecuteTool,
		suppressedWaitTool,
		commandlessExecuteWithError,
	];

	it("collapses consecutive read_file tool blocks", () => {
		const result = toTimelineBlocks(
			[
				{ type: "tool", id: "read-1" },
				{ type: "tool", id: "read-2" },
			],
			tools,
		);

		expect(result).toEqual([
			{ type: "read-files", tools: [tool("read-1"), tool("read-2")] },
		]);
	});

	it("emits a single read_file tool block as a one-file group", () => {
		const result = toTimelineBlocks([{ type: "tool", id: "read-1" }], tools);

		expect(result).toEqual([{ type: "read-files", tools: [tool("read-1")] }]);
	});

	it.each([
		[
			"response content",
			[
				{ type: "tool", id: "read-1" },
				{ type: "response", text: "middle" },
				{ type: "tool", id: "read-2" },
			],
			[
				{ type: "read-files", tools: [tool("read-1")] },
				{ type: "response", text: "middle", sourceIndex: 1 },
				{ type: "read-files", tools: [tool("read-2")] },
			],
		],
		[
			"another tool",
			[
				{ type: "tool", id: "read-1" },
				{ type: "tool", id: "execute-1" },
				{ type: "tool", id: "read-2" },
			],
			[
				{ type: "read-files", tools: [tool("read-1")] },
				{ type: "tool", tool: executeTool("execute-1") },
				{ type: "read-files", tools: [tool("read-2")] },
			],
		],
		[
			"an unresolved tool, which becomes its own block",
			[
				{ type: "tool", id: "read-1" },
				{ type: "tool", id: "missing" },
				{ type: "tool", id: "read-2" },
			],
			[
				{ type: "read-files", tools: [tool("read-1")] },
				{ type: "unresolved-tool", id: "missing" },
				{ type: "read-files", tools: [tool("read-2")] },
			],
		],
		[
			"a tool with neither a command nor a result yet",
			[
				{ type: "tool", id: "read-1" },
				{ type: "tool", id: "execute-pending" },
				{ type: "tool", id: "read-2" },
			],
			[
				{ type: "read-files", tools: [tool("read-1")] },
				{ type: "unresolved-tool", id: "execute-pending" },
				{ type: "read-files", tools: [tool("read-2")] },
			],
		],
	] satisfies Array<
		[string, RenderBlock[], ReturnType<typeof toTimelineBlocks>]
	>)("does not collapse read_file blocks across %s", (_, blocks, expected) => {
		expect(toTimelineBlocks(blocks, tools)).toEqual(expected);
	});

	it("keeps a non-tool block's source index after a run collapses", () => {
		const result = toTimelineBlocks(
			[
				{ type: "tool", id: "read-1" },
				{ type: "tool", id: "read-2" },
				{ type: "thinking", text: "hmm" },
			],
			tools,
		);

		expect(result).toEqual([
			{ type: "read-files", tools: [tool("read-1"), tool("read-2")] },
			{ type: "thinking", text: "hmm", sourceIndex: 2 },
		]);
	});

	it("emits a block whose tool has not arrived yet as unresolved", () => {
		expect(toTimelineBlocks([{ type: "tool", id: "missing" }], tools)).toEqual([
			{ type: "unresolved-tool", id: "missing" },
		]);
	});

	// A wait_agent row without its target chat_id is deliberately hidden by
	// <Tool>, not still arriving, so it must not become a placeholder.
	it("keeps a suppressed subagent lifecycle tool as a tool block", () => {
		expect(toTimelineBlocks([{ type: "tool", id: "wait-1" }], tools)).toEqual([
			{ type: "tool", tool: suppressedWaitTool },
		]);
	});

	// A settled execute with no command carries the error explaining why, so it
	// must reach <Tool> rather than sit behind a placeholder forever.
	it("keeps an execute whose result explains its missing command", () => {
		expect(
			toTimelineBlocks([{ type: "tool", id: "execute-failed" }], tools),
		).toEqual([{ type: "tool", tool: commandlessExecuteWithError }]);
	});

	it("hands two tools sharing one id to one block each", () => {
		const first = { ...tool("dup"), args: { path: "a.ts" } };
		const second = { ...tool("dup"), args: { path: "b.ts" } };

		expect(
			toTimelineBlocks(
				[
					{ type: "tool", id: "dup" },
					{ type: "tool", id: "dup" },
				],
				[first, second],
			),
		).toEqual([{ type: "read-files", tools: [first, second] }]);
	});
});
