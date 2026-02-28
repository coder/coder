import { describe, expect, it } from "vitest";
import {
	applyMessagePartToStreamState,
	applyStreamThinkingTitle,
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

describe("applyStreamThinkingTitle", () => {
	it("returns blocks unchanged when title is undefined", () => {
		const blocks = [{ type: "response" as const, text: "hello" }];
		expect(applyStreamThinkingTitle(blocks, undefined)).toBe(blocks);
	});

	it("creates a new thinking block when last block is not thinking", () => {
		const blocks = [{ type: "response" as const, text: "hello" }];
		const result = applyStreamThinkingTitle(blocks, "Plan");
		expect(result).toHaveLength(2);
		expect(result[1]).toEqual({ type: "thinking", text: "", title: "Plan" });
	});

	it("merges title into existing thinking block", () => {
		const blocks = [
			{ type: "thinking" as const, text: "some thought", title: "Old" },
		];
		const result = applyStreamThinkingTitle(blocks, "Old and more");
		expect(result).toHaveLength(1);
		expect(result[0]).toEqual({
			type: "thinking",
			text: "some thought",
			title: "Old and more",
		});
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
		expect(ids[0]).toBe("tool-call-1");
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
		};
		const tools = buildStreamTools(state);
		expect(tools).toHaveLength(1);
		expect(tools[0].status).toBe("completed");
	});
});
