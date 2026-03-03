import { describe, expect, it } from "vitest";
import {
	mergeTools,
	normalizeBlockType,
	parseMessageContent,
	parseToolResultIsError,
} from "./messageParsing";

describe("normalizeBlockType", () => {
	it("lowercases and replaces underscores with hyphens", () => {
		expect(normalizeBlockType("Tool_Call")).toBe("tool-call");
		expect(normalizeBlockType("TOOL_RESULT")).toBe("tool-result");
	});

	it("returns empty string for non-string input", () => {
		expect(normalizeBlockType(undefined)).toBe("");
		expect(normalizeBlockType(null)).toBe("");
	});
});

describe("parseToolResultIsError", () => {
	it("returns the boolean is_error when present", () => {
		expect(parseToolResultIsError("tool", { is_error: true }, null)).toBe(true);
		expect(parseToolResultIsError("tool", { is_error: false }, null)).toBe(
			false,
		);
	});

	it("returns false when no error indicator is present", () => {
		expect(parseToolResultIsError("tool", {}, null)).toBe(false);
	});

	it("returns true when error field is present for non-subagent tools", () => {
		expect(
			parseToolResultIsError("some_tool", { error: "something" }, null),
		).toBe(true);
	});

	it("returns false for completed subagent even with error field", () => {
		expect(
			parseToolResultIsError(
				"spawn_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"wait_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
		expect(
			parseToolResultIsError(
				"message_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
	});
});

describe("parseMessageContent", () => {
	it("returns empty result for null content", () => {
		const result = parseMessageContent(null);
		expect(result.markdown).toBe("");
		expect(result.blocks).toEqual([]);
		expect(result.toolCalls).toEqual([]);
		expect(result.toolResults).toEqual([]);
	});

	it("returns empty result for undefined content", () => {
		const result = parseMessageContent(undefined);
		expect(result.markdown).toBe("");
		expect(result.blocks).toEqual([]);
	});

	it("returns empty result for an empty array", () => {
		const result = parseMessageContent([]);
		expect(result.markdown).toBe("");
		expect(result.blocks).toEqual([]);
		expect(result.toolCalls).toEqual([]);
		expect(result.toolResults).toEqual([]);
	});

	it("handles a plain string content", () => {
		const result = parseMessageContent("Hello world");
		expect(result.markdown).toBe("Hello world");
		expect(result.blocks).toEqual([]);
	});

	it("parses a single text block", () => {
		const result = parseMessageContent([{ type: "text", text: "Hello" }]);
		expect(result.markdown).toBe("Hello");
		expect(result.blocks).toEqual([{ type: "response", text: "Hello" }]);
	});

	it("merges multiple text blocks into a single response block", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Line one" },
			{ type: "text", text: "Line two" },
		]);
		expect(result.markdown).toBe("Line one\nLine two");
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Line one\nLine two",
		});
	});

	it("parses a thinking block", () => {
		const result = parseMessageContent([
			{ type: "thinking", text: "Let me think...", title: "Reasoning" },
		]);
		expect(result.reasoning).toBe("Let me think...");
		expect(result.blocks).toEqual([
			{ type: "thinking", text: "Let me think...", title: "Reasoning" },
		]);
	});

	it("parses a tool_use / tool-call block", () => {
		const result = parseMessageContent([
			{
				type: "tool-call",
				tool_name: "bash",
				tool_call_id: "call-1",
				args: { command: "ls" },
			},
		]);
		expect(result.toolCalls).toHaveLength(1);
		expect(result.toolCalls[0]).toEqual({
			id: "call-1",
			name: "bash",
			args: { command: "ls" },
		});
		expect(result.blocks).toEqual([{ type: "tool", id: "call-1" }]);
	});

	it("parses a tool-result block", () => {
		const result = parseMessageContent([
			{
				type: "tool-result",
				tool_name: "bash",
				tool_call_id: "call-1",
				result: { output: "file.txt" },
			},
		]);
		expect(result.toolResults).toHaveLength(1);
		expect(result.toolResults[0]).toEqual({
			id: "call-1",
			name: "bash",
			result: { output: "file.txt" },
			isError: false,
		});
		expect(result.blocks).toEqual([{ type: "tool", id: "call-1" }]);
	});

	it("handles interleaved text and tool blocks in correct order", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Starting..." },
			{
				type: "tool-call",
				tool_name: "bash",
				tool_call_id: "call-1",
				args: {},
			},
			{
				type: "tool-result",
				tool_name: "bash",
				tool_call_id: "call-1",
				result: "ok",
			},
			{ type: "text", text: "Done!" },
		]);
		expect(result.blocks).toHaveLength(3);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Starting...",
		});
		expect(result.blocks[1]).toEqual({ type: "tool", id: "call-1" });
		// The second text block creates a new response block after the
		// tool block.
		expect(result.blocks[2]).toEqual({ type: "response", text: "Done!" });
	});

	it("generates fallback IDs when tool_call_id is missing", () => {
		const result = parseMessageContent([
			{ type: "tool-call", tool_name: "run" },
		]);
		expect(result.toolCalls[0].id).toBe("tool-call-0");
	});

	it("handles unknown block types gracefully (no crash)", () => {
		const result = parseMessageContent([
			{ type: "unknown_block_type", text: "some text" },
		]);
		// Unknown types fall through to the default branch which treats
		// the text field as a response.
		expect(result.markdown).toBe("some text");
		expect(result.blocks).toEqual([{ type: "response", text: "some text" }]);
	});

	it("handles non-object array entries gracefully", () => {
		const result = parseMessageContent(["raw string", 42, null]);
		expect(result.markdown).toBe("raw string");
		expect(result.blocks).toEqual([{ type: "response", text: "raw string" }]);
	});

	it("handles an object with a type field (treated as single-element array)", () => {
		const result = parseMessageContent({ type: "text", text: "single" });
		expect(result.markdown).toBe("single");
	});

	it("handles an object with text/content fields", () => {
		const result = parseMessageContent({ text: "fallback text" });
		expect(result.markdown).toBe("fallback text");
	});

	it("normalizes underscore block types like tool_call", () => {
		const result = parseMessageContent([
			{
				type: "tool_call",
				tool_name: "test",
				tool_call_id: "tc-1",
				args: {},
			},
		]);
		expect(result.toolCalls).toHaveLength(1);
		expect(result.toolCalls[0].name).toBe("test");
	});
});

describe("mergeTools", () => {
	it("merges tool calls with matching results", () => {
		const merged = mergeTools(
			[{ id: "1", name: "bash", args: { cmd: "ls" } }],
			[{ id: "1", name: "bash", result: "ok", isError: false }],
		);
		expect(merged).toHaveLength(1);
		expect(merged[0]).toEqual({
			id: "1",
			name: "bash",
			args: { cmd: "ls" },
			result: "ok",
			isError: false,
			status: "completed",
		});
	});

	it("includes orphaned results that have no matching call", () => {
		const merged = mergeTools(
			[],
			[{ id: "1", name: "bash", result: "output", isError: false }],
		);
		expect(merged).toHaveLength(1);
		expect(merged[0].id).toBe("1");
		expect(merged[0].status).toBe("completed");
	});

	it("marks error results with error status", () => {
		const merged = mergeTools(
			[{ id: "1", name: "bash" }],
			[{ id: "1", name: "bash", result: "fail", isError: true }],
		);
		expect(merged[0].isError).toBe(true);
		expect(merged[0].status).toBe("error");
	});

	it("returns completed for calls without results", () => {
		const merged = mergeTools([{ id: "1", name: "bash" }], []);
		expect(merged).toHaveLength(1);
		expect(merged[0].status).toBe("completed");
	});
});
