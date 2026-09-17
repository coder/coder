import type { Chat } from "#/api/typesGenerated";
import { getGroupLabel } from "./boardLabels";

/** Members keyed by the id of the primary they render under. */
export type BoardGroups = ReadonlyMap<string, readonly Chat[]>;

/** True when the chat belongs to a board group led by another chat. */
export const isBoardGroupMember = (chat: Chat): boolean =>
	getGroupLabel(chat) !== chat.id;

/**
 * Board groups drawn in one list: members keyed by their primary, only when
 * both are in `chats`. Members whose primary is elsewhere stay where they
 * are, so grouping never removes a chat from view.
 */
export const collectBoardGroups = (chats: readonly Chat[]): BoardGroups => {
	const ids = new Set(chats.map((chat) => chat.id));
	const members = new Map<string, Chat[]>();
	for (const chat of chats) {
		const primaryID = getGroupLabel(chat);
		if (primaryID === chat.id || !ids.has(primaryID)) continue;
		const list = members.get(primaryID) ?? [];
		list.push(chat);
		members.set(primaryID, list);
	}
	return members;
};

/**
 * What the sidebar lists with the board on: members leave their own slot
 * and render inside their primary's entry. Off, the list is returned as
 * given and `groups` is null, so the sidebar renders as if the board did
 * not exist.
 */
export const boardSidebarChats = (
	chats: readonly Chat[],
	enabled: boolean,
): { chats: readonly Chat[]; groups: BoardGroups | null } => {
	if (!enabled) return { chats, groups: null };
	const groups = collectBoardGroups(chats);
	const memberIDs = new Set([...groups.values()].flat().map((chat) => chat.id));
	return { chats: chats.filter((chat) => !memberIDs.has(chat.id)), groups };
};
