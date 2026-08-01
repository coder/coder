import { arrayMove } from "@dnd-kit/sortable";
import type { Chat } from "#/api/typesGenerated";

type PinnedChatDropArgs = {
	/** Pinned chats in the order the sidebar is rendering them. */
	pinnedChats: readonly Chat[];
	/**
	 * True when the rendered order comes from the panel's own drag
	 * override rather than straight from the chats prop.
	 */
	hasLocalOrder: boolean;
	activeId: string;
	overId: string;
};

type PinnedChatDrop = {
	/** Chat ids in the post-drop order, for the panel's local override. */
	localOrder: string[];
	chatId: string;
	pinOrder: number;
	/** The pre-drop pinned chats the reorder mutation renumbers against. */
	mutationChats: readonly Chat[];
};

/**
 * Resolves a pinned-chat drop into the panel's next local order and the
 * reorder mutation's payload. Returns undefined when either end of the
 * drop is not a pinned chat.
 *
 * While a local drag order is active, the chats still carry the pin_order
 * fields of the prop the panel last rendered with, which lags behind the
 * drops already applied to the cache. The mutation reads those fields to
 * build the snapshot it rolls back to, so it is handed copies renumbered
 * to their rendered positions instead. The pre-drop order is what gets
 * renumbered: normalizing the post-drop order would make the rollback
 * snapshot equal the optimistic one and turn rollback into a no-op.
 *
 * Without a local order the fields come from the cache and can hold
 * legitimate server-assigned gaps, so they are passed through untouched.
 */
export const resolvePinnedChatDrop = ({
	pinnedChats,
	hasLocalOrder,
	activeId,
	overId,
}: PinnedChatDropArgs): PinnedChatDrop | undefined => {
	const pinnedChatIds = pinnedChats.map((chat) => chat.id);
	const oldIndex = pinnedChatIds.indexOf(activeId);
	const newIndex = pinnedChatIds.indexOf(overId);
	if (oldIndex === -1 || newIndex === -1) {
		return undefined;
	}

	return {
		localOrder: arrayMove(pinnedChatIds, oldIndex, newIndex),
		chatId: activeId,
		pinOrder: newIndex + 1,
		mutationChats: hasLocalOrder
			? pinnedChats.map((chat, index) => ({ ...chat, pin_order: index + 1 }))
			: pinnedChats,
	};
};
