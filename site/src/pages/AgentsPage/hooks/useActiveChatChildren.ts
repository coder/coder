import { useInfiniteQuery } from "react-query";
import {
	type ChatListInput,
	infiniteProjectFolderChats,
} from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { useCachedChat } from "./useCachedChat";

/**
 * Finds the active chat's children in the chat list caches, which watch
 * events keep fresh. The sidebar's list leaves out the user's own project
 * chats, so the chat may only be in its project folder's list. The chat
 * entity's fetch-time snapshot comes last, so a subagent that was running
 * still blocks archiving when no list holds the chat.
 */
export const useActiveChatChildren = (
	chatId: string | undefined,
	sidebarChats: readonly Chat[],
	chatListInput: ChatListInput,
): readonly Chat[] | undefined => {
	const cachedChat = useCachedChat(chatId);
	const folderQuery = useInfiniteQuery({
		...infiniteProjectFolderChats(cachedChat?.project_id ?? "", chatListInput),
		enabled: false,
	});
	const listedChat =
		sidebarChats.find((c) => c.id === chatId) ??
		folderQuery.data?.pages.flat().find((c) => c.id === chatId);
	return (listedChat ?? cachedChat)?.children;
};
