import type { Chat } from "#/api/typesGenerated";
import type { ChatTree } from "./chatTree";

interface VisibleChatSection {
	readonly key: string;
	readonly chats: readonly Chat[];
}

/**
 * Returns chat IDs in the order they appear in the sidebar.
 *
 * `visible` omits chats in collapsed sections and children of roots
 * that are not expanded. `all` keeps every chat in the same order,
 * regardless of collapse state.
 */
export const getVisibleChatOrder = ({
	sections,
	collapsedSections,
	expandedById,
	tree,
}: {
	readonly sections: readonly VisibleChatSection[];
	readonly collapsedSections: Record<string, boolean>;
	readonly expandedById: Record<string, boolean>;
	readonly tree: ChatTree;
}): { visible: string[]; all: string[] } => {
	const visible: string[] = [];
	const all: string[] = [];
	for (const section of sections) {
		const sectionVisible = !collapsedSections[section.key];
		for (const chat of section.chats) {
			all.push(chat.id);
			if (sectionVisible) {
				visible.push(chat.id);
			}
			const childrenVisible = sectionVisible && Boolean(expandedById[chat.id]);
			for (const childID of tree.childrenById.get(chat.id) ?? []) {
				all.push(childID);
				if (childrenVisible) {
					visible.push(childID);
				}
			}
		}
	}
	return { visible, all };
};
