import type { ChatInputPart } from "#/api/typesGenerated";
import { MaxChatMCPAppContextBytes } from "#/api/typesGenerated";
import type {
	ChatStore,
	McpAppContextEntry,
} from "../ChatConversation/chatStore";

type ModelContextBlock = {
	type: string;
	text?: string;
};

type ModelContextParams = {
	content?: readonly ModelContextBlock[];
	structuredContent?: Record<string, unknown>;
};

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/** Cuts a string at the UTF-8 byte cap without splitting a code point. */
const truncateUtf8 = (text: string, maxBytes: number): string => {
	const bytes = encoder.encode(text);
	if (bytes.length <= maxBytes) {
		return text;
	}
	// The decoder drops an incomplete trailing sequence instead of emitting a
	// replacement character when the input is cut mid code point.
	return decoder.decode(bytes.subarray(0, maxBytes), { stream: true });
};

/**
 * Flattens a `ui/update-model-context` payload into the single text field
 * the chat input part carries. Text blocks are joined with newlines and
 * structured content is appended as JSON.
 */
export const flattenModelContext = (params: ModelContextParams): string => {
	const lines: string[] = [];
	for (const block of params.content ?? []) {
		if (block.type === "text" && typeof block.text === "string") {
			lines.push(block.text);
		}
	}
	if (params.structuredContent !== undefined) {
		lines.push(JSON.stringify(params.structuredContent));
	}
	return truncateUtf8(lines.join("\n"), MaxChatMCPAppContextBytes);
};

export const buildMcpAppContextInputParts = (
	entries: readonly McpAppContextEntry[],
): ChatInputPart[] =>
	entries
		.filter(([, context]) => context.text.length > 0)
		.map(([, context]) => ({
			type: "mcp-app-context",
			mcp_server_config_id: context.mcpServerConfigId,
			mcp_app_resource_uri: context.resourceUri,
			text: context.text,
		}));

/**
 * Removes the pending app contexts from the store and returns them as input
 * parts together with a `restore` that puts them back if the send fails.
 * Edits and slash commands never carry app context, so those callers get no
 * parts and the store is left untouched.
 */
export const takeMcpAppContextInputParts = (
	store: Pick<ChatStore, "takeMcpAppContexts" | "restoreMcpAppContexts">,
	{ isNewSend }: { isNewSend: boolean },
): { parts: ChatInputPart[]; restore: () => void } => {
	if (!isNewSend) {
		return { parts: [], restore: () => {} };
	}
	const entries = store.takeMcpAppContexts();
	return {
		parts: buildMcpAppContextInputParts(entries),
		restore: () => store.restoreMcpAppContexts(entries),
	};
};
