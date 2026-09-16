import type { ChatStatus } from "#/api/typesGenerated";
import { isActiveChatStatus } from "../ChatConversation/chatStore";
import type { MergedTool } from "../ChatConversation/types";

type McpAppPhase =
	| "idle"
	| "loading_resource"
	| "booting"
	| "initialized"
	| "torn_down"
	| "error";

export type McpAppLifecycleState = {
	phase: McpAppPhase;
	/** Incremented on every rebind so callbacks from a stale boot are ignored. */
	generation: number;
	boundToolCallId?: string;
	/** True once the view HTML has been fetched for this tab. */
	resourceLoaded: boolean;
	error?: string;
};

export type McpAppLifecycleAction =
	| { type: "bind"; toolCallId: string }
	| { type: "resourceLoaded" }
	| { type: "sandboxReady" }
	| { type: "initialized" }
	| { type: "tornDown" }
	| { type: "fail"; message: string }
	| { type: "reset" };

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
		case "bind": {
			if (state.boundToolCallId === action.toolCallId) {
				return state;
			}
			const rebinding = state.boundToolCallId !== undefined;
			return {
				...state,
				boundToolCallId: action.toolCallId,
				generation: rebinding ? state.generation + 1 : state.generation,
				phase: state.resourceLoaded ? "booting" : "loading_resource",
				error: undefined,
			};
		}
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
		case "tornDown":
			return state.phase === "initialized"
				? { ...state, phase: "torn_down" }
				: state;
		case "fail":
			return { ...state, phase: "error", error: action.message };
		case "reset":
			return {
				...initialMcpAppLifecycleState,
				generation: state.generation + 1,
			};
		default: {
			const _exhaustive: never = action;
			return state;
		}
	}
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

/**
 * Builds a spec-shaped CallToolResult from the model-facing result when the
 * raw MCP result was omitted or truncated.
 */
const synthesizeToolResult = (
	result: unknown,
	isError: boolean,
): Record<string, unknown> => {
	const text = typeof result === "string" ? result : JSON.stringify(result);
	return {
		content: [{ type: "text", text: text ?? "" }],
		...(isRecord(result) ? { structuredContent: result } : {}),
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
	const args = isRecord(tool.args) ? tool.args : undefined;
	const hasResult = tool.result !== undefined || tool.mcpResult !== undefined;
	const result = hasResult
		? isRecord(tool.mcpResult)
			? tool.mcpResult
			: synthesizeToolResult(tool.result, tool.isError)
		: undefined;
	return {
		args,
		argsComplete: !tool.argsStreaming && (args !== undefined || hasResult),
		result,
		cancelled: !hasResult && !isActiveChatStatus(chatStatus),
	};
};
