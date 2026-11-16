import { useQuery } from "react-query";
import { chat } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";

/**
 * Reads a chat that another view already loaded, such as the chat page's
 * active chat, without fetching it.
 */
export const useCachedChat = (chatId: string | undefined): Chat | undefined =>
	useQuery({ ...chat(chatId ?? ""), enabled: false }).data;
