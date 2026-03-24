import { describe, expect, it } from "vitest";
import {
	applyMessagePartToStreamState,
	buildStreamTools,
	createEmptyStreamState,
} from "./streamState";
import type { StreamState } from "./types";

describe("createEmptyStreamState", () => {
	it("returns fresh state with empty blocks and tool maps", () => {
		const state = createEmptyStreamState();
		expect(state.blocks).toEqual([]);
		expect(state.toolCalls).toEqual({});
		expect(state.toolResults).toEqual({});
	});
});

describe("applyMessagePartToStreamState", () => {
	it("creates new state with response block from text part on null prev", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "text",
			text: "Hello",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks).toEqual([{ type: "response", text: "Hello" }]);
	});

	it("appends text to existing response block", () => {
		const prev: StreamState = {
			blocks: [{ type: "response", text: "Hello" }],
			toolCalls: {},
			toolResults: {},
			sources: [],
		};
		const result = applyMessagePartToStreamState(prev, {
			type: "text",
			text: " world",
		});
		expect(result!.blocks).toHaveLength(1);
		expect(result!.blocks[0]).toEqual({
			type: "response",
			text: "Hello world",
		});
	});

	it("returns prev when text part has empty text", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "text",
			text: "",
		});
		expect(result).toBe(prev);
	});

	it("creates thinking block from reasoning part", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "reasoning",
			text: "Let me reason...",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks).toEqual([
			{ type: "thinking", text: "Let me reason..." },
		]);
	});

	it("returns prev for reasoning part with empty text", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "reasoning",
			text: "",
		});
		expect(result).toBe(prev);
	});

	it("creates tool call entry from tool-call part", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "tool-call",
			tool_name: "bash",
			tool_call_id: "tc-1",
			args: { command: "ls" },
		});
		expect(result).not.toBeNull();
		expect(result!.toolCalls["tc-1"]).toEqual({
			id: "tc-1",
			name: "bash",
			args: { command: "ls" },
			argsRaw: undefined,
		});
		expect(result!.blocks).toEqual([{ type: "tool", id: "tc-1" }]);
	});

	it("generates fallback tool call ID when missing", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "tool-call",
			tool_name: "run",
		});
		expect(result).not.toBeNull();
		const ids = Object.keys(result!.toolCalls);
		expect(ids).toHaveLength(1);
		expect(ids[0]).toMatch(/^tool-call-1-\d+$/);
	});

	it("does not collide IDs for multiple tool calls with the same name", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			args: { command: "ls" },
		});
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			args: { command: "pwd" },
		});
		const ids = Object.keys(state!.toolCalls);
		expect(ids).toHaveLength(2);
		// The two calls must have distinct IDs.
		expect(ids[0]).not.toBe(ids[1]);
		// Each call must retain its own args.
		const calls = Object.values(state!.toolCalls);
		const args = calls.map((c) => c.args);
		expect(args).toContainEqual({ command: "ls" });
		expect(args).toContainEqual({ command: "pwd" });
	});

	it("does not collide IDs for multiple tool results with the same name", () => {
		let state: StreamState | null = null;
		// Two tool calls with the same name, each receiving a separate result.
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			args: { command: "ls" },
		});
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			args: { command: "pwd" },
		});
		const callIds = Object.keys(state!.toolCalls);
		expect(callIds).toHaveLength(2);

		// First result arrives without a tool_call_id.
		state = applyMessagePartToStreamState(state, {
			type: "tool-result",
			tool_name: "bash",
			result: { output: "file.txt" },
		});
		// Second result arrives without a tool_call_id.
		state = applyMessagePartToStreamState(state, {
			type: "tool-result",
			tool_name: "bash",
			result: { output: "/home" },
		});

		const resultIds = Object.keys(state!.toolResults);
		expect(resultIds).toHaveLength(2);
		// The two results must have distinct IDs.
		expect(resultIds[0]).not.toBe(resultIds[1]);
		// Each result must retain its own output.
		const results = Object.values(state!.toolResults);
		const outputs = results.map((r) => r.result);
		expect(outputs).toContainEqual({ output: "file.txt" });
		expect(outputs).toContainEqual({ output: "/home" });
	});

	it("creates tool result entry from tool-result part", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "tool-result",
			tool_name: "bash",
			tool_call_id: "tc-1",
			result: { output: "file.txt" },
		});
		expect(result).not.toBeNull();
		expect(result!.toolResults["tc-1"]).toMatchObject({
			id: "tc-1",
			name: "bash",
			result: { output: "file.txt" },
			isError: false,
		});
	});

	it("accumulates multiple tool calls in sequence", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			tool_call_id: "tc-1",
			args: { cmd: "ls" },
		});
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "read",
			tool_call_id: "tc-2",
			args: { path: "/tmp" },
		});
		expect(Object.keys(state!.toolCalls)).toHaveLength(2);
		expect(state!.blocks).toHaveLength(2);
	});

	it("accumulates args via args_delta across multiple tool-call parts", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_call_id: "tc-1",
			tool_name: "bash",
			args_delta: '{"com',
		});
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_call_id: "tc-1",
			tool_name: "bash",
			args_delta: 'mand":"ls"}',
		});
		expect(state).not.toBeNull();
		expect(state!.toolCalls["tc-1"].args).toEqual({ command: "ls" });
		expect(state!.toolCalls["tc-1"].argsRaw).toBe('{"command":"ls"}');
	});

	it("accepts complete args without args_delta", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_call_id: "tc-1",
			tool_name: "bash",
			args: { command: "ls" },
		});
		expect(state).not.toBeNull();
		expect(state!.toolCalls["tc-1"].args).toEqual({ command: "ls" });
	});

	it("skips provider_executed tool-call parts", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "tool-call",
			tool_name: "web_search",
			tool_call_id: "tc-1",
			provider_executed: true,
		});
		expect(result).toBe(prev);
		expect(prev.toolCalls).toEqual({});
	});

	it("skips provider_executed tool-result parts", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "tool-result",
			tool_name: "web_search",
			tool_call_id: "tc-1",
			provider_executed: true,
			result: { output: "search results" },
		});
		expect(result).toBe(prev);
		expect(prev.toolResults).toEqual({});
	});

	it("adds a file block from a file part with data", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "file",
			media_type: "image/png",
			data: "iVBORw0KGgo=",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks).toHaveLength(1);
		expect(result!.blocks[0]).toMatchObject({
			type: "file",
			media_type: "image/png",
			data: "iVBORw0KGgo=",
		});
	});

	it("adds a file block from a file part with file_id", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "file",
			media_type: "image/png",
			file_id: "abc-123",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks).toHaveLength(1);
		expect(result!.blocks[0]).toMatchObject({
			type: "file",
			media_type: "image/png",
			file_id: "abc-123",
		});
	});

	it("returns prev for file part without data or file_id", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "file",
			media_type: "image/png",
		});
		expect(result).toBe(prev);
	});

	it("returns prev for file-reference part (not a streaming type)", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "file-reference",
			file_name: "main.go",
			start_line: 1,
			end_line: 10,
			content: "package main",
		});
		expect(result).toBe(prev);
	});

	it("adds a sources block from a source part", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "source",
			url: "https://example.com",
			title: "Example",
		});
		expect(state).not.toBeNull();
		expect(state!.sources).toEqual([
			{ url: "https://example.com", title: "Example" },
		]);
		expect(state!.blocks).toHaveLength(1);
		expect(state!.blocks[0]).toEqual({
			type: "sources",
			sources: [{ url: "https://example.com", title: "Example" }],
		});

		// A second source with a different URL groups into the same block.
		state = applyMessagePartToStreamState(state, {
			type: "source",
			url: "https://other.com",
			title: "Other",
		});
		expect(state!.sources).toHaveLength(2);
		expect(state!.blocks).toHaveLength(1);
		expect(state!.blocks[0]).toEqual({
			type: "sources",
			sources: [
				{ url: "https://example.com", title: "Example" },
				{ url: "https://other.com", title: "Other" },
			],
		});
	});

	it("deduplicates sources with the same URL", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "source",
			url: "https://example.com",
			title: "Example",
		});
		const afterFirst = state;
		state = applyMessagePartToStreamState(state, {
			type: "source",
			url: "https://example.com",
			title: "Example",
		});
		// Second application returns prev unchanged.
		expect(state).toBe(afterFirst);
		expect(state!.sources).toHaveLength(1);
	});

	it("produces correct tool-result shape with is_error through buildStreamTools", () => {
		let state: StreamState | null = null;
		state = applyMessagePartToStreamState(state, {
			type: "tool-call",
			tool_name: "bash",
			tool_call_id: "tc-1",
			args: { command: "rm -rf /" },
		});
		state = applyMessagePartToStreamState(state, {
			type: "tool-result",
			tool_name: "bash",
			tool_call_id: "tc-1",
			result: { error: "permission denied" },
			is_error: true,
		});
		expect(state).not.toBeNull();
		expect(state!.toolResults["tc-1"]).toMatchObject({
			id: "tc-1",
			name: "bash",
			result: { error: "permission denied" },
			isError: true,
		});
		const tools = buildStreamTools(state);
		expect(tools).toHaveLength(1);
		expect(tools[0]).toEqual({
			id: "tc-1",
			name: "bash",
			args: { command: "rm -rf /" },
			result: { error: "permission denied" },
			isError: true,
			status: "error",
		});
	});
});

describe("buildStreamTools", () => {
	it("returns empty array for null stream state", () => {
		expect(buildStreamTools(null)).toEqual([]);
	});

	it("returns running status for calls without results", () => {
		const state: StreamState = {
			blocks: [{ type: "tool", id: "tc-1" }],
			toolCalls: {
				"tc-1": { id: "tc-1", name: "bash", args: { cmd: "ls" } },
			},
			toolResults: {},
			sources: [],
		};
		const tools = buildStreamTools(state);
		expect(tools).toHaveLength(1);
		expect(tools[0].status).toBe("running");
	});

	it("returns completed status when call has a result", () => {
		const state: StreamState = {
			blocks: [],
			toolCalls: {
				"tc-1": { id: "tc-1", name: "bash" },
			},
			toolResults: {
				"tc-1": {
					id: "tc-1",
					name: "bash",
					result: "ok",
					isError: false,
				},
			},
			sources: [],
		};
		const tools = buildStreamTools(state);
		expect(tools[0].status).toBe("completed");
	});

	it("includes orphan results with no matching call", () => {
		const state: StreamState = {
			blocks: [],
			toolCalls: {},
			toolResults: {
				"tc-1": {
					id: "tc-1",
					name: "bash",
					result: "output",
					isError: false,
				},
			},
			sources: [],
		};
		const tools = buildStreamTools(state);
		expect(tools).toHaveLength(1);
		expect(tools[0].status).toBe("completed");
	});
});
