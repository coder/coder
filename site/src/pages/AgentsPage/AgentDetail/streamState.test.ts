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

	it("creates thinking block from thinking part", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "thinking",
			text: "Let me reason...",
			title: "Analysis",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks).toEqual([
			{ type: "thinking", text: "Let me reason...", title: "Analysis" },
		]);
	});

	it("handles reasoning type alias the same as thinking", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "reasoning",
			text: "hmm",
		});
		expect(result).not.toBeNull();
		expect(result!.blocks[0].type).toBe("thinking");
	});

	it("returns prev for thinking part with no text and no title", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "thinking",
			text: "",
		});
		expect(result).toBe(prev);
	});

	it("returns prev for thinking part with only title and no text", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "thinking",
			text: "",
			title: "Some Title",
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

		// First result arrives without an explicit tool_call_id.
		state = applyMessagePartToStreamState(state, {
			type: "tool-result",
			tool_name: "bash",
			result: { output: "file.txt" },
		});
		// Second result arrives without an explicit tool_call_id.
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

	it("handles tool_call underscore type alias", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "tool_call",
			tool_name: "test",
			tool_call_id: "t1",
		});
		expect(result).not.toBeNull();
		expect(result!.toolCalls.t1).toBeDefined();
	});

	it("returns prev for unknown part type", () => {
		const prev = createEmptyStreamState();
		const result = applyMessagePartToStreamState(prev, {
			type: "banana",
		});
		expect(result).toBe(prev);
	});

	it("returns null for unknown part type when prev is null", () => {
		const result = applyMessagePartToStreamState(null, {
			type: "banana",
		});
		expect(result).toBeNull();
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
