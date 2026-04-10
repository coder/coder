import { useMemo } from "react";
import type { Chat, ChatMessage } from "#/api/typesGenerated";

export function deriveChatSummary(
	chat: Chat,
	messages: readonly ChatMessage[],
): string {
	const parts: string[] = [];

	// Status-based summary
	switch (chat.status) {
		case "completed":
			parts.push("Agent finished its work.");
			break;
		case "waiting":
			parts.push("Agent is waiting for your input.");
			break;
		case "error":
			parts.push(
				`Agent encountered an error${chat.last_error ? `: ${chat.last_error}` : "."}`,
			);
			break;
		case "running":
			parts.push("Agent is still working.");
			break;
		case "pending":
			parts.push("Agent is starting up.");
			break;
		case "paused":
			parts.push("Agent is paused.");
			break;
		case "requires_action":
			parts.push("Agent requires your action.");
			break;
	}

	// Count tool calls
	const toolCalls = messages.filter((m) => m.role === "tool").length;
	if (toolCalls > 0) {
		parts.push(`Used ${toolCalls} tool${toolCalls === 1 ? "" : "s"}.`);
	}

	// Diff status
	if (chat.diff_status) {
		parts.push("Has pending code changes to review.");
	}

	return parts.join(" ");
}

export function useChatSummary(
	chat: Chat | undefined,
	messages: readonly ChatMessage[],
): string {
	return useMemo(() => {
		if (!chat) {
			return "";
		}
		return deriveChatSummary(chat, messages);
	}, [chat, messages]);
}
