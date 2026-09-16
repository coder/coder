import type * as TypesGen from "#/api/typesGenerated";
import type { ReconnectSchedule } from "#/utils/reconnectingWebSocket";

export type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
	parsedCommands?: readonly string[][];
	mcpServerConfigId?: string;
	mcpAppResourceUri?: string;
	hookRewritten?: boolean;
};

export type ParsedToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
	mcpServerConfigId?: string;
	mcpAppResourceUri?: string;
	/** Raw MCP CallToolResult for tools that render an MCP App. */
	mcpResult?: unknown;
	mcpResultTruncated?: boolean;
};

export type MergedTool = {
	id: string;
	name: string;
	args?: unknown;
	result?: unknown;
	isError: boolean;
	status: "completed" | "error" | "running";
	mcpServerConfigId?: string;
	mcpAppResourceUri?: string;
	/** Raw MCP CallToolResult for tools that render an MCP App. */
	mcpResult?: unknown;
	mcpResultTruncated?: boolean;
	/** True while the streamed args are still accumulating from deltas. */
	argsStreaming?: boolean;
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	hookRewritten?: boolean;
	/** Set when a process_signal killed/terminated this process. */
	killedBySignal?: "kill" | "terminate";
};

export type RenderBlock =
	| {
			type: "response";
			text: string;
	  }
	| {
			type: "thinking";
			text: string;
	  }
	| {
			type: "tool";
			id: string;
	  }
	| TypesGen.ChatFilePart
	| TypesGen.ChatFileReferencePart
	| {
			type: "sources";
			sources: Array<{ url: string; title: string }>;
	  };

export type ParsedMessageContent = {
	markdown: string;
	reasoning: string;
	toolCalls: ParsedToolCall[];
	toolResults: ParsedToolResult[];
	tools: MergedTool[];
	blocks: RenderBlock[];
	sources: Array<{ url: string; title: string }>;
	hookNotices: string[];
	mcpAppContexts: TypesGen.ChatMcpAppContextPart[];
};

export type ParsedMessageEntry = {
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	// IDs of the messages folded into this entry when consecutive read_file
	// runs are merged into one row. Absent for ordinary messages.
	mergedFrom?: readonly number[];
};

export type ReconnectState = ReconnectSchedule;

export type RetryState = {
	attempt: number;
	error: string;
	kind: TypesGen.ChatErrorKind;
	provider?: string;
	retryingAt?: string;
};

type StreamToolCall = {
	id: string;
	name: string;
	args?: unknown;
	argsRaw?: string;
	parsedCommands?: readonly string[][];
	mcpServerConfigId?: string;
	mcpAppResourceUri?: string;
	modelIntent?: string;
};

type StreamToolResult = {
	id: string;
	name: string;
	result?: unknown;
	resultRaw?: string;
	isError: boolean;
	/** True while result deltas are still accumulating before the final result. */
	isStreaming?: boolean;
	mcpServerConfigId?: string;
	mcpAppResourceUri?: string;
	mcpResult?: unknown;
	mcpResultTruncated?: boolean;
};

export type StreamState = {
	blocks: RenderBlock[];
	toolCalls: Record<string, StreamToolCall>;
	toolResults: Record<string, StreamToolResult>;
	sources: Array<{ url: string; title: string }>;
};
