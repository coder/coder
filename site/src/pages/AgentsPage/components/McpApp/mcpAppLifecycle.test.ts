import { describe, expect, it } from "vitest";
import type { MergedTool } from "../ChatConversation/types";
import {
	deriveBoundCall,
	initialMcpAppLifecycleState,
	type McpAppLifecycleAction,
	type McpAppLifecycleState,
	mcpAppLifecycleReducer,
} from "./mcpAppLifecycle";

const run = (
	actions: readonly McpAppLifecycleAction[],
	start: McpAppLifecycleState = initialMcpAppLifecycleState,
): McpAppLifecycleState => actions.reduce(mcpAppLifecycleReducer, start);

describe("mcpAppLifecycleReducer", () => {
	it("walks idle -> loading_resource -> booting -> initialized -> torn_down", () => {
		const bound = run([{ type: "bind", toolCallId: "call-1" }]);
		expect(bound).toMatchObject({
			phase: "loading_resource",
			boundToolCallId: "call-1",
			generation: 0,
		});

		const booting = run([{ type: "resourceLoaded" }], bound);
		expect(booting).toMatchObject({ phase: "booting", resourceLoaded: true });

		const initialized = run(
			[{ type: "sandboxReady" }, { type: "initialized" }],
			booting,
		);
		expect(initialized.phase).toBe("initialized");

		expect(run([{ type: "tornDown" }], initialized).phase).toBe("torn_down");
	});

	it("ignores initialized outside of booting", () => {
		expect(run([{ type: "initialized" }]).phase).toBe("idle");
	});

	it("returns the same state when the resource is reported loaded again", () => {
		const loaded = run([
			{ type: "bind", toolCallId: "call-1" },
			{ type: "resourceLoaded" },
		]);
		expect(mcpAppLifecycleReducer(loaded, { type: "resourceLoaded" })).toBe(
			loaded,
		);
	});

	it("rebinding to another tool call bumps the generation", () => {
		const first = run([
			{ type: "bind", toolCallId: "call-1" },
			{ type: "resourceLoaded" },
			{ type: "sandboxReady" },
			{ type: "initialized" },
		]);
		const rebound = run([{ type: "bind", toolCallId: "call-2" }], first);
		expect(rebound).toMatchObject({
			boundToolCallId: "call-2",
			generation: 1,
			// The HTML was already fetched, so the view goes straight to boot.
			phase: "booting",
		});
	});

	it("rebinding before the resource loaded stays in loading_resource", () => {
		const state = run([
			{ type: "bind", toolCallId: "call-1" },
			{ type: "bind", toolCallId: "call-2" },
		]);
		expect(state).toMatchObject({
			phase: "loading_resource",
			generation: 1,
			boundToolCallId: "call-2",
		});
	});

	it("binding the same tool call again is a no-op", () => {
		const state = run([{ type: "bind", toolCallId: "call-1" }]);
		expect(
			mcpAppLifecycleReducer(state, { type: "bind", toolCallId: "call-1" }),
		).toBe(state);
	});

	it("records failures and clears them on rebind", () => {
		const failed = run([
			{ type: "bind", toolCallId: "call-1" },
			{ type: "fail", message: "boom" },
		]);
		expect(failed).toMatchObject({ phase: "error", error: "boom" });
		const rebound = run([{ type: "bind", toolCallId: "call-2" }], failed);
		expect(rebound.error).toBeUndefined();
		expect(rebound.phase).toBe("loading_resource");
	});

	it("reset returns to idle with a new generation", () => {
		const state = run([
			{ type: "bind", toolCallId: "call-1" },
			{ type: "bind", toolCallId: "call-2" },
			{ type: "reset" },
		]);
		expect(state).toEqual({ ...initialMcpAppLifecycleState, generation: 2 });
	});
});

describe("deriveBoundCall", () => {
	const tool = (overrides: Partial<MergedTool>): MergedTool => ({
		id: "call-1",
		name: "add_task",
		isError: false,
		status: "completed",
		...overrides,
	});

	it("returns undefined when the tool call is not loaded", () => {
		expect(deriveBoundCall("missing", [tool({})], "waiting")).toBeUndefined();
		expect(deriveBoundCall(undefined, [tool({})], "waiting")).toBeUndefined();
	});

	it("prefers the raw MCP result over the model-facing result", () => {
		const mcpResult = { content: [{ type: "text", text: "ok" }] };
		const bound = deriveBoundCall(
			"call-1",
			[tool({ args: { title: "x" }, result: "ok", mcpResult })],
			"waiting",
		);
		expect(bound).toEqual({
			args: { title: "x" },
			argsComplete: true,
			result: mcpResult,
			cancelled: false,
		});
	});

	it("synthesizes a CallToolResult when the raw result was truncated", () => {
		const bound = deriveBoundCall(
			"call-1",
			[
				tool({
					args: {},
					result: { tasks: [] },
					mcpResultTruncated: true,
				}),
			],
			"waiting",
		);
		expect(bound?.result).toEqual({
			content: [{ type: "text", text: '{"tasks":[]}' }],
			structuredContent: { tasks: [] },
		});
	});

	it("marks error results as isError when synthesizing", () => {
		const bound = deriveBoundCall(
			"call-1",
			[tool({ args: {}, result: "failed", isError: true, status: "error" })],
			"waiting",
		);
		expect(bound?.result).toEqual({
			content: [{ type: "text", text: "failed" }],
			isError: true,
		});
	});

	it("reports streaming args as incomplete", () => {
		const bound = deriveBoundCall(
			"call-1",
			[tool({ args: { tit: "" }, status: "running", argsStreaming: true })],
			"running",
		);
		expect(bound).toEqual({
			args: { tit: "" },
			argsComplete: false,
			result: undefined,
			cancelled: false,
		});
	});

	it("is cancelled only when the chat stopped without a result", () => {
		const running = deriveBoundCall(
			"call-1",
			[tool({ args: {}, status: "running" })],
			"running",
		);
		expect(running?.cancelled).toBe(false);

		const stopped = deriveBoundCall(
			"call-1",
			[tool({ args: {}, status: "completed" })],
			"waiting",
		);
		expect(stopped?.cancelled).toBe(true);
	});
});
