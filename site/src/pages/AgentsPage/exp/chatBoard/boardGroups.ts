import type { Chat } from "#/api/typesGenerated";
import { getGroupLabel } from "./boardLabels";

/** True when the chat belongs to a board group led by another chat. */
export const isBoardGroupMember = (chat: Chat): boolean =>
	getGroupLabel(chat) !== chat.id;

/**
 * Board groups drawn in one list: members keyed by their primary, only when
 * both are in `chats`. Members whose primary is elsewhere stay where they
 * are, so grouping never removes a chat from view.
 */
export const collectBoardGroups = (
	chats: readonly Chat[],
): ReadonlyMap<string, readonly Chat[]> => {
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
