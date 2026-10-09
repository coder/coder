import type * as TypesGen from "#/api/typesGenerated";
import type { ReconnectSchedule } from "#/utils/reconnectingWebSocket";

export type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
	parsedCommands?: readonly string[][];
	mcpServerConfigId?: string;
	hookRewritten?: boolean;
	startedAt?: string;
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
};

export type RenderBlock =
	| {
			type: "response";
			text: string;
			/**
			 * A provider-executed call, such as web search, followed this text,
			 * so later text starts a new block.
			 */
			beforeProviderTool?: boolean;
			/** The model labeled this text as commentary on its work. */
			narration?: boolean;
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
	| TypesGen.ChatWorkspaceFileReferencePart
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
	startedAt?: string;
	/**
	 * Set by a provider-executed result, so a search counts as a step even
	 * before or without citations.
	 */
	providerToolRan?: boolean;
};
