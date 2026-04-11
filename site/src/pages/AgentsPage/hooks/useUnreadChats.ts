import { useMemo } from "react";
import type { Chat } from "#/api/typesGenerated";

interface UseUnreadChatsResult {
	unreadChats: Chat[];
	unreadCount: number;
}

export function useUnreadChats(
	chatList: readonly Chat[],
): UseUnreadChatsResult {
	return useMemo(() => {
		const unreadChats = chatList.filter(
			(chat) => chat.has_unread && !chat.parent_chat_id,
		);
		return {
			unreadChats,
			unreadCount: unreadChats.length,
		};
	}, [chatList]);
}
