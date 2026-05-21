import type { Chat } from "#/api/typesGenerated";
import { asNonEmptyString } from "../../ChatConversation/blockUtils";

export type ChatTree = {
	readonly rootIds: readonly string[];
	readonly chatById: ReadonlyMap<string, Chat>;
	readonly childrenById: ReadonlyMap<string, readonly string[]>;
	readonly parentById: ReadonlyMap<string, string | undefined>;
};

export const getParentChatID = (chat: Chat): string | undefined => {
	return asNonEmptyString(chat.parent_chat_id);
};

export const buildChatTree = (chats: readonly Chat[]): ChatTree => {
	const chatById = new Map<string, Chat>();
	const parentById = new Map<string, string | undefined>();
	const childrenById = new Map<string, string[]>();

	// The paginated list now contains only root chats. Children
	// are embedded in each root's `children` field.
	for (const chat of chats) {
		chatById.set(chat.id, chat);
		childrenById.set(chat.id, []);
		// Guard against stale cache entries: if a flat child
		// entry appears in `chats` after its embedded parent has
		// already set its parent link, do not overwrite the link
		// with `undefined`. Without this, the defensive fallback
		// below re-adds the child to its parent's list, producing
		// a duplicate key in React rendering.
		if (!parentById.has(chat.id)) {
			parentById.set(chat.id, undefined);
		}

		if (chat.children) {
			for (const child of chat.children) {
				chatById.set(child.id, child);
				parentById.set(child.id, chat.id);
				childrenById.get(chat.id)?.push(child.id);
				// Children cannot have their own children (depth
				// capped at 1), but initialize the map entry for
				// uniform lookup.
				childrenById.set(child.id, []);
			}
		}
	}

	// Defensive fallback for cached data during rollout: if any
	// chat has a parent_chat_id that points to a chat in the list
	// but was not embedded, build the link. This handles stale
	// cache entries from before the backend change.
	for (const chat of chats) {
		const parentID = getParentChatID(chat);
		if (
			parentID &&
			parentID !== chat.id &&
			chatById.has(parentID) &&
			!parentById.get(chat.id)
		) {
			parentById.set(chat.id, parentID);
			childrenById.get(parentID)?.push(chat.id);
		}
	}

	const rootIds = chats
		.map((chat) => chat.id)
		.filter((chatID) => !parentById.get(chatID));

	return {
		rootIds,
		chatById,
		childrenById,
		parentById,
	};
};

export const collectVisibleChatIDs = ({
	chats,
	search,
	tree,
}: {
	readonly chats: readonly Chat[];
	readonly search: string;
	readonly tree: ChatTree;
}): Set<string> => {
	if (!search) {
		const allIDs = new Set(chats.map((chat) => chat.id));
		for (const chat of chats) {
			for (const child of chat.children ?? []) {
				allIDs.add(child.id);
			}
		}
		return allIDs;
	}

	const allChats = chats.flatMap((chat) => [chat, ...(chat.children ?? [])]);
	const matchedChatIDs = allChats
		.filter((chat) => chat.title.toLowerCase().includes(search))
		.map((chat) => chat.id);
	if (matchedChatIDs.length === 0) {
		return new Set<string>();
	}

	const visible = new Set<string>();
	for (const matchedChatID of matchedChatIDs) {
		let parentCursor: string | undefined = matchedChatID;
		const seenParents = new Set<string>();
		while (parentCursor && !seenParents.has(parentCursor)) {
			seenParents.add(parentCursor);
			visible.add(parentCursor);
			parentCursor = tree.parentById.get(parentCursor);
		}

		const stack = [matchedChatID];
		const seenDescendants = new Set<string>();
		while (stack.length > 0) {
			const currentID = stack.pop();
			if (!currentID || seenDescendants.has(currentID)) {
				continue;
			}
			seenDescendants.add(currentID);
			visible.add(currentID);
			for (const childID of tree.childrenById.get(currentID) ?? []) {
				stack.push(childID);
			}
		}
	}

	return visible;
};
