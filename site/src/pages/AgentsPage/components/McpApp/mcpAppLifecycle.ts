import type { ChatStatus } from "#/api/typesGenerated";
import { isActiveChatStatus } from "../ChatConversation/chatStore";
import type { MergedTool } from "../ChatConversation/types";
import { asRecord } from "../ChatElements/runtimeTypeUtils";

type McpAppPhase =
	| "idle"
	| "loading_resource"
	| "booting"
	| "initialized"
	| "error";

export type McpAppLifecycleState = {
	phase: McpAppPhase;
	/** Incremented on every rebind so the iframe remounts and stale boots are ignored. */
	generation: number;
	/** True once the view HTML has been fetched for this tab. */
	resourceLoaded: boolean;
	error?: string;
};

export type McpAppLifecycleAction =
	| { type: "bind" }
	| { type: "resourceLoaded" }
	| { type: "sandboxReady" }
	| { type: "initialized" }
	| { type: "fail"; message: string };

export const initialMcpAppLifecycleState: McpAppLifecycleState = {
	phase: "idle",
	generation: 0,
	resourceLoaded: false,
};

export const mcpAppLifecycleReducer = (
	state: McpAppLifecycleState,
	action: McpAppLifecycleAction,
): McpAppLifecycleState => {
	switch (action.type) {
		case "bind":
			return {
				...state,
				// Only the first bind keeps the generation: later binds replace a
				// view that already started and must reload the iframe.
				generation:
					state.phase === "idle" ? state.generation : state.generation + 1,
				phase: state.resourceLoaded ? "booting" : "loading_resource",
				error: undefined,
			};
		case "resourceLoaded":
			if (state.phase === "loading_resource") {
				return { ...state, resourceLoaded: true, phase: "booting" };
			}
			return state.resourceLoaded ? state : { ...state, resourceLoaded: true };
		case "sandboxReady":
			return state.phase === "booting" ? state : { ...state, phase: "booting" };
		case "initialized":
			return state.phase === "booting"
				? { ...state, phase: "initialized" }
				: state;
		case "fail":
			return { ...state, phase: "error", error: action.message };
		default: {
			const _exhaustive: never = action;
			return state;
		}
	}
};

/**
 * Builds a spec-shaped CallToolResult from the model-facing result when the
 * raw MCP result was omitted or truncated.
 */
const synthesizeToolResult = (
	result: unknown,
	isError: boolean,
): Record<string, unknown> => {
	const text = typeof result === "string" ? result : JSON.stringify(result);
	const structured = asRecord(result);
	return {
		content: [{ type: "text", text: text ?? "" }],
		...(structured ? { structuredContent: structured } : {}),
		...(isError ? { isError: true } : {}),
	};
};

export type BoundCall = {
	args?: Record<string, unknown>;
	/** False while tool-call args are still streaming as deltas. */
	argsComplete: boolean;
	result?: Record<string, unknown>;
	/** The chat stopped without ever producing a result for this call. */
	cancelled: boolean;
};

export const deriveBoundCall = (
	toolCallId: string | undefined,
	tools: readonly MergedTool[],
	chatStatus: ChatStatus | null,
): BoundCall | undefined => {
	const tool = tools.find((candidate) => candidate.id === toolCallId);
	if (!tool) {
		return undefined;
	}
	const args = asRecord(tool.args) ?? undefined;
	const hasResult = tool.result !== undefined || tool.mcpResult !== undefined;
	const mcpResult = asRecord(tool.mcpResult);
	const result = hasResult
		? (mcpResult ?? synthesizeToolResult(tool.result, tool.isError))
		: undefined;
	return {
		args,
		argsComplete: !tool.argsStreaming && (args !== undefined || hasResult),
		result,
		cancelled: !hasResult && !isActiveChatStatus(chatStatus),
	};
};
