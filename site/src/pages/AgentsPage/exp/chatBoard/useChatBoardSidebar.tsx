import type { ReactNode } from "react";
import type { Chat } from "#/api/typesGenerated";
import { ChatTreeNode } from "../../components/ChatsSidebar/tree/ChatTreeNode";
import { BoardGroupEntry } from "./BoardGroupEntry";
import { collectBoardGroups, isBoardGroupMember } from "./boardGroups";
import { getColumnLabel, INBOX_COLUMN } from "./boardLabels";
import { ColumnTag } from "./ColumnTag";
import { useChatBoardEnabled } from "./useChatBoardEnabled";

interface ChatBoardSidebar {
	/** The list to render: `chats` minus group members drawn in a box. */
	readonly chats: readonly Chat[];
	/** Right-column slot for every row under the tree context, or undefined. */
	readonly renderTrailing: ((chat: Chat) => ReactNode) | undefined;
	/** One keyed list entry for a chat in `chats`. */
	readonly renderEntry: (chat: Chat) => ReactNode;
}

const renderPlainEntry = (chat: Chat): ReactNode => (
	<ChatTreeNode key={chat.id} chat={chat} />
);

/**
 * What the board adds to the sidebar chat list. With the flag off every
 * value is the plain list behaviour, so ChatsPanel renders as if the board
 * did not exist.
 */
export const useChatBoardSidebar = (
	chats: readonly Chat[],
): ChatBoardSidebar => {
	const [enabled] = useChatBoardEnabled();
	if (!enabled) {
		return { chats, renderTrailing: undefined, renderEntry: renderPlainEntry };
	}

	// Members render inside their primary's box, so they leave their own
	// slot only when the primary is in this same list.
	const groups = collectBoardGroups(chats);
	const memberIDs = new Set([...groups.values()].flat().map((chat) => chat.id));
	return {
		chats: chats.filter((chat) => !memberIDs.has(chat.id)),
		// A group box names the column once in its header, so its primary and
		// members carry no tag. A primary with members in this list is a
		// root, owned, unpinned chat and therefore only ever rendered in
		// its box.
		renderTrailing: (chat) => {
			const column = getColumnLabel(chat);
			if (
				column === INBOX_COLUMN ||
				isBoardGroupMember(chat) ||
				groups.has(chat.id)
			) {
				return null;
			}
			return <ColumnTag name={column} />;
		},
		renderEntry: (chat) => (
			<BoardGroupEntry
				key={chat.id}
				chat={chat}
				members={groups.get(chat.id)}
			/>
		),
	};
};
