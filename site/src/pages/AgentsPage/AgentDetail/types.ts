import type * as TypesGen from "api/typesGenerated";

export type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
};

export type ParsedToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
};

export type MergedTool = {
	id: string;
	name: string;
	args?: unknown;
	result?: unknown;
	isError: boolean;
	status: "completed" | "error" | "running";
};

export type RenderBlock =
	| {
			type: "response";
			text: string;
	  }
	| {
			type: "thinking";
			text: string;
			title?: string;
	  }
	| {
			type: "tool";
			id: string;
	  };

export type ParsedMessageContent = {
	markdown: string;
	reasoning: string;
	toolCalls: ParsedToolCall[];
	toolResults: ParsedToolResult[];
	tools: MergedTool[];
	blocks: RenderBlock[];
};

export type ParsedMessageEntry = {
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
};

export type ParsedMessageSection = {
	userEntry: ParsedMessageEntry | null;
	entries: ParsedMessageEntry[];
};

type StreamToolCall = {
	id: string;
	name: string;
	args?: unknown;
	argsRaw?: string;
};

type StreamToolResult = {
	id: string;
	name: string;
	result?: unknown;
	resultRaw?: string;
	isError: boolean;
};

export type StreamState = {
	blocks: RenderBlock[];
	toolCalls: Record<string, StreamToolCall>;
	toolResults: Record<string, StreamToolResult>;
};
