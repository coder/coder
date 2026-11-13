import type { Chat } from "#/api/typesGenerated";
import { canManageChat } from "../components/ChatActionsMenuItems";

/**
 * Pin order is the owner's sidebar preference and the server ranks it per
 * owner, so another user's chat is never pinned for the viewer, whatever its
 * `pin_order`.
 */
export const isOwnPinnedChat = (chat: Chat, currentUserId: string): boolean =>
	canManageChat(chat, currentUserId) && chat.pin_order > 0;

/** The viewer's own pinned chats, in pin order. */
export const getOwnPinnedChats = (
	chats: readonly Chat[],
	currentUserId: string,
): Chat[] =>
	chats
		.filter((chat) => isOwnPinnedChat(chat, currentUserId))
		.sort((a, b) => a.pin_order - b.pin_order);
