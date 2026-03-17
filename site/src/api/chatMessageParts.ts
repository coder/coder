// Discriminated union variants for ChatMessagePart.
//
// The generated ChatMessagePart in typesGenerated.ts is a flat
// interface where all fields besides `type` are optional. These
// variants narrow the fields to what's actually present for each
// part type, enabling TypeScript discriminated union narrowing
// when switching on `type`.
//
// Manually maintained to match the Go constructors in
// codersdk/chats.go. Update these if the Go type changes.
//
// Import ChatMessagePart from this module instead of
// typesGenerated to get discriminated union narrowing.

import type {
	ChatMessage as GeneratedChatMessage,
	ChatQueuedMessage as GeneratedChatQueuedMessage,
	ChatStreamMessagePart as GeneratedChatStreamMessagePart,
} from "api/typesGenerated";

/** Text content in a chat message. */
export interface ChatTextPart {
	readonly type: "text";
	readonly text: string;
	/** Present on cryptographically signed messages. */
	readonly signature?: string;
}

/** Model reasoning/thinking content, shown in a collapsible block. */
export interface ChatReasoningPart {
	readonly type: "reasoning";
	readonly text: string;
}

/** A tool invocation requested by the model. */
export interface ChatToolCallPart {
	readonly type: "tool-call";
	readonly tool_call_id: string;
	readonly tool_name: string;
	readonly args?: Record<string, string>;
	/** Streaming delta for args, appended incrementally. */
	readonly args_delta?: string;
	/** True when the provider executed the tool (e.g. web search). */
	readonly provider_executed?: boolean;
}

/** The result of a tool invocation. */
export interface ChatToolResultPart {
	readonly type: "tool-result";
	readonly tool_call_id: string;
	readonly tool_name: string;
	readonly result?: Record<string, string>;
	/** Streaming delta for result, appended incrementally. */
	readonly result_delta?: string;
	readonly is_error?: boolean;
}

/** An uploaded file (typically an image). */
export interface ChatFilePart {
	readonly type: "file";
	readonly media_type: string;
	/** Base64-encoded content, absent when file_id is available. */
	readonly data?: string;
	readonly file_id?: string;
}

/** A reference to a file region (e.g. from a diff annotation). */
export interface ChatFileReferencePart {
	readonly type: "file-reference";
	readonly file_name: string;
	readonly start_line: number;
	readonly end_line: number;
	readonly content: string;
}

/** A citation source returned by web search or RAG. */
export interface ChatSourcePart {
	readonly type: "source";
	readonly source_id?: string;
	readonly url: string;
	readonly title?: string;
}

/**
 * Discriminated union of all chat message part types.
 *
 * Import this as ChatMessagePart instead of the generated flat
 * interface to get automatic type narrowing on `part.type`.
 */
export type ChatMessagePart =
	| ChatTextPart
	| ChatReasoningPart
	| ChatToolCallPart
	| ChatToolResultPart
	| ChatFilePart
	| ChatFileReferencePart
	| ChatSourcePart;

// Type guards for use with .filter() and conditionals.
// TypeScript narrows automatically in if/switch blocks, but
// .filter() requires an explicit type predicate.

export const isTextPart = (p: ChatMessagePart): p is ChatTextPart =>
	p.type === "text";

export const isFilePart = (p: ChatMessagePart): p is ChatFilePart =>
	p.type === "file";

export const isFileReferencePart = (
	p: ChatMessagePart,
): p is ChatFileReferencePart => p.type === "file-reference";

export const isToolCallPart = (p: ChatMessagePart): p is ChatToolCallPart =>
	p.type === "tool-call";

export const isToolResultPart = (p: ChatMessagePart): p is ChatToolResultPart =>
	p.type === "tool-result";

export const isSourcePart = (p: ChatMessagePart): p is ChatSourcePart =>
	p.type === "source";

/**
 * Narrowed ChatMessage with discriminated union content.
 * Import this instead of the generated ChatMessage to get
 * automatic type narrowing on message.content items.
 */
export interface ChatMessage extends Omit<GeneratedChatMessage, "content"> {
	readonly content?: readonly ChatMessagePart[];
}

/**
 * Narrowed ChatQueuedMessage with discriminated union content.
 */
export interface ChatQueuedMessage
	extends Omit<GeneratedChatQueuedMessage, "content"> {
	readonly content: readonly ChatMessagePart[];
}

/**
 * Narrowed ChatStreamMessagePart with discriminated union part.
 */
export interface ChatStreamMessagePart
	extends Omit<GeneratedChatStreamMessagePart, "part"> {
	readonly part: ChatMessagePart;
}
