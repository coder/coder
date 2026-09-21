import type {
	ACPSession,
	ChatMessage,
	ChatMessagePart,
} from "#/api/typesGenerated";
import { parseMessagesWithMergedTools } from "../ChatConversation/messageParsing";

/** Projects ephemeral ACP entries into the shared conversation's display format. */
export function acpTranscript(session: ACPSession) {
	const messages: ChatMessage[] = session.entries.map((entry, index) => {
		const content: ChatMessagePart[] = [];
		if (entry.kind === "tool") {
			const name = entry.title || "Tool";
			content.push({
				type: "tool-call",
				tool_call_id: entry.id,
				tool_name: name,
			});
			if (entry.status === "completed" || entry.status === "failed") {
				content.push({
					type: "tool-result",
					tool_call_id: entry.id,
					tool_name: name,
					result: entry.text || entry.output,
					is_error: entry.status === "failed",
				});
			}
		} else if (entry.kind === "reasoning")
			content.push({ type: "reasoning", text: entry.text });
		else content.push({ type: "text", text: entry.text });
		return {
			id: index + 1,
			chat_id: session.session_id,
			created_at: session.created_at,
			role: entry.role === "user" ? "user" : "assistant",
			content,
		};
	});
	const byId = new Map(session.entries.map((entry) => [entry.id, entry]));
	const pendingToolCallIDs = new Set(
		session.entries
			.filter(
				(entry) =>
					entry.kind === "tool" &&
					entry.status !== "completed" &&
					entry.status !== "failed",
			)
			.map((entry) => entry.id),
	);
	const parsed = parseMessagesWithMergedTools(messages, { pendingToolCallIDs });
	for (const message of parsed) {
		for (const tool of message.parsed.tools)
			tool.args = byId.get(tool.id)?.input;
		for (const call of message.parsed.toolCalls)
			call.args = byId.get(call.id)?.input;
	}
	return parsed;
}
