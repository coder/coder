import type { ChatMCPApp, ChatMessage } from "#/api/typesGenerated";

type McpAppToolResult = {
	messageId: number;
	toolCallId: string;
	app: ChatMCPApp;
};

/** The persisted tool results that carry MCP Apps view data. */
export function* mcpAppToolResults(
	messages: Iterable<ChatMessage>,
): Generator<McpAppToolResult> {
	for (const message of messages) {
		for (const part of message.content ?? []) {
			if (part.type === "tool-result" && part.tool_call_id && part.mcp_app) {
				yield {
					messageId: message.id,
					toolCallId: part.tool_call_id,
					app: part.mcp_app,
				};
			}
		}
	}
}
