import type { Chat } from "#/api/typesGenerated";

/**
 * Status-based grouping utility used by the sidebar to categorize
 * chats into semantic status buckets.
 */
export const CHAT_STATUS_GROUPS = [
	"Running",
	"Unread",
	"Error",
	"Idle/awaiting feedback",
	"Archived",
] as const;

type ChatStatusGroup = (typeof CHAT_STATUS_GROUPS)[number];

export function getChatStatusGroup(chat: Chat): ChatStatusGroup {
	if (chat.archived) {
		return "Archived";
	}
	if (chat.status === "pending" || chat.status === "running") {
		return "Running";
	}
	if (chat.has_unread) {
		return "Unread";
	}
	if (chat.status === "error") {
		return "Error";
	}
	return "Idle/awaiting feedback";
}
