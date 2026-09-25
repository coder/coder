import type * as TypesGen from "#/api/typesGenerated";
import type { ReconnectSchedule } from "#/utils/reconnectingWebSocket";

/** A URL shown as a pill, either a found page or an answer citation. */
export type SourceLink = { url: string; title: string };

export type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
	parsedCommands?: readonly string[][];
	mcpServerConfigId?: string;
	hookRewritten?: boolean;
	startedAt?: string;
	providerExecuted?: boolean;
	/** Pages a provider-executed web search returned, from tagged source parts. */
	foundPages?: SourceLink[];
};

export type ParsedToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
	isMedia?: boolean;
	mcpServerConfigId?: string;
};

export type MergedTool = {
	id: string;
	name: string;
	args?: unknown;
	result?: unknown;
	/** Streamed advisor reasoning, present only while the advisor runs. */
	reasoning?: string;
	isError: boolean;
	isMedia?: boolean;
	status: "completed" | "error" | "running";
	mcpServerConfigId?: string;
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	hookRewritten?: boolean;
	/** Set when a process_signal killed/terminated this process. */
	killedBySignal?: "kill" | "terminate";
	/** When the model emitted the call, from the tool-call part's created_at. */
	startedAt?: string;
	providerExecuted?: boolean;
	foundPages?: readonly SourceLink[];
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
	modelIntent?: string;
	startedAt?: string;
	providerExecuted?: boolean;
	foundPages?: readonly SourceLink[];
};

type StreamToolResult = {
	id: string;
	name: string;
	result?: unknown;
	resultRaw?: string;
	/** Reasoning deltas so far; absent when none arrived or the result is final. */
	reasoning?: string;
	isError: boolean;
	isMedia?: boolean;
	/** True while result or reasoning deltas are still accumulating before the final result. */
	isStreaming?: boolean;
	mcpServerConfigId?: string;
};

export type StreamState = {
	blocks: RenderBlock[];
	toolCalls: Record<string, StreamToolCall>;
	toolResults: Record<string, StreamToolResult>;
	sources: Array<{ url: string; title: string }>;
};
