import { useMemo } from "react";
import type { Chat } from "#/api/typesGenerated";

const REVIEW_THRESHOLD = 3;

interface UseUnreadChatsResult {
	unreadChats: Chat[];
	unreadCount: number;
	hasReviewThreshold: boolean;
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
			hasReviewThreshold: unreadChats.length >= REVIEW_THRESHOLD,
		};
	}, [chatList]);
}
