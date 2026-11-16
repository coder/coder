import { useInfiniteQuery, useQuery } from "react-query";
import {
	type ChatListInput,
	cachedChat,
	infiniteProjectFolderChats,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";

/**
 * Reads the active chat's children from the sidebar's list, then from its
 * project folder's list, which holds the user's own project chats, then
 * from the chat entity's snapshot.
 */
export const useActiveChatChildren = (
	chatId: string | undefined,
	sidebarChats: readonly Chat[],
	chatListInput: ChatListInput,
): readonly Chat[] | undefined => {
	const { data: entity } = useQuery(cachedChat(chatId));
	const folderQuery = useInfiniteQuery({
		...infiniteProjectFolderChats(entity?.project_id ?? "", chatListInput),
		enabled: false,
	});
	return (
		sidebarChats.find((c) => c.id === chatId) ??
		folderQuery.data?.pages.flat().find((c) => c.id === chatId) ??
		entity
	)?.children;
};
