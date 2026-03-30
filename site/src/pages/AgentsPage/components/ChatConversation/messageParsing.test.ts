import { describe, expect, it } from "vitest";
import {
	mergeTools,
	parseMessageContent,
	parseToolResultIsError,
} from "./messageParsing";

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
		expect(
			parseToolResultIsError(
				"spawn_computer_use_agent",
				{ error: "metadata" },
				{ status: "completed" },
			),
		).toBe(false);
	});
});

describe("parseMessageContent", () => {
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
		expect(result.markdown).toBe("Line oneLine two");
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Line oneLine two",
		});
	});

	it("normalizes list markers split across text blocks", () => {
		// LLMs stream list markers and item content as separate text
		// blocks. Both paths (streaming and completed) must concatenate
		// them directly so the marker and content stay on the same line.
		const result = parseMessageContent([
			{ type: "text", text: "Intro\n\n- " },
			{ type: "text", text: "First item" },
			{ type: "text", text: "\n- " },
			{ type: "text", text: "Second item" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "response",
			text: "Intro\n\n- First item\n- Second item",
		});
	});

	it("parses a reasoning block", () => {
		const result = parseMessageContent([
			{ type: "reasoning", text: "Let me think..." },
		]);
		expect(result.reasoning).toBe("Let me think...");
		expect(result.blocks).toEqual([
			{ type: "thinking", text: "Let me think..." },
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
				result: { output: "ok" },
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

	it("extracts fileId from a file block with file_id", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
				file_id: "abc-123-def",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file",
			media_type: "image/png",
			data: undefined,
			file_id: "abc-123-def",
		});
	});

	it("parses a file block without file_id (backward compat)", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
				data: "iVBORw0KGgo=",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file",
			media_type: "image/png",
			data: "iVBORw0KGgo=",
			file_id: undefined,
		});
	});

	it("skips file parts without data or file_id", () => {
		const result = parseMessageContent([
			{
				type: "file",
				media_type: "image/png",
			},
		]);
		expect(result.blocks).toHaveLength(0);
	});

	it("parses a file-reference block into blocks", () => {
		const result = parseMessageContent([
			{
				type: "file-reference",
				file_name: "src/main.go",
				start_line: 10,
				end_line: 15,
				content: "some added code lines",
			},
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "file-reference",
			file_name: "src/main.go",
			start_line: 10,
			end_line: 15,
			content: "some added code lines",
		});
	});

	it("does not affect markdown when file-reference blocks are present", () => {
		const result = parseMessageContent([
			{ type: "text", text: "Hello" },
			{
				type: "file-reference",
				file_name: "a.go",
				start_line: 1,
				end_line: 2,
				content: "nit code content",
			},
		]);
		expect(result.markdown).toBe("Hello");
		expect(result.blocks).toHaveLength(2);
		expect(result.blocks[0]).toEqual({ type: "response", text: "Hello" });
		expect(result.blocks[1]).toEqual({
			type: "file-reference",
			file_name: "a.go",
			start_line: 1,
			end_line: 2,
			content: "nit code content",
		});
	});

	it("skips provider_executed tool-call parts", () => {
		const result = parseMessageContent([
			{
				type: "tool-call",
				tool_name: "web_search",
				tool_call_id: "tc-1",
				provider_executed: true,
			},
		]);
		expect(result.toolCalls).toEqual([]);
		expect(result.blocks.some((b) => b.type === "tool")).toBe(false);
	});

	it("skips provider_executed tool-result parts", () => {
		const result = parseMessageContent([
			{
				type: "tool-result",
				tool_name: "web_search",
				tool_call_id: "tc-1",
				provider_executed: true,
				result: { output: "results" },
			},
		]);
		expect(result.toolResults).toEqual([]);
		expect(result.blocks.some((b) => b.type === "tool")).toBe(false);
	});

	it("parses a source part into a sources block", () => {
		const result = parseMessageContent([
			{ type: "source", url: "https://example.com", title: "Example" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "sources",
			sources: [{ url: "https://example.com", title: "Example" }],
		});
		expect(result.sources).toEqual([
			{ url: "https://example.com", title: "Example" },
		]);
	});

	it("groups multiple consecutive sources into a single sources block", () => {
		const result = parseMessageContent([
			{ type: "source", url: "https://example.com", title: "Example" },
			{ type: "source", url: "https://other.com", title: "Other" },
		]);
		expect(result.blocks).toHaveLength(1);
		expect(result.blocks[0]).toEqual({
			type: "sources",
			sources: [
				{ url: "https://example.com", title: "Example" },
				{ url: "https://other.com", title: "Other" },
			],
		});
		expect(result.sources).toHaveLength(2);
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
