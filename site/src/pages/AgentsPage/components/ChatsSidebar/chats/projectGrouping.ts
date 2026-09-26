import type { Chat, ChatProject } from "#/api/typesGenerated";

type ChatsGroupedByProject = {
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly unfiledChats: readonly Chat[];
};

/**
 * Splits chats into their project folders. A chat whose project is not in
 * `projects` (unloaded, another organization, or not owned) stays unfiled so
 * it never disappears from the sidebar.
 */
export const groupChatsByProject = (
	chats: readonly Chat[],
	projects: readonly ChatProject[],
): ChatsGroupedByProject => {
	const loadedProjectIds = new Set(projects.map((project) => project.id));
	const chatsByProjectId = new Map<string, Chat[]>();
	const unfiledChats: Chat[] = [];
	for (const chat of chats) {
		if (chat.project_id && loadedProjectIds.has(chat.project_id)) {
			const bucket = chatsByProjectId.get(chat.project_id);
			if (bucket) {
				bucket.push(chat);
			} else {
				chatsByProjectId.set(chat.project_id, [chat]);
			}
		} else {
			unfiledChats.push(chat);
		}
	}
	return { chatsByProjectId, unfiledChats };
};
