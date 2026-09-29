import type { Chat, ChatProject } from "#/api/typesGenerated";

type ChatsGroupedByProject = {
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly unfiledChats: readonly Chat[];
};

/**
 * Splits chats into their project folders. While projects are loading, chats
 * with a project are held back so they do not jump from the unfiled list into
 * a folder. Once loaded, a chat whose project is not in `projects` (another
 * organization, not owned, or the request failed) stays unfiled so it never
 * disappears from the sidebar.
 */
export const groupChatsByProject = (
	chats: readonly Chat[],
	projects: readonly ChatProject[],
	isLoadingProjects = false,
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
		} else if (!chat.project_id || !isLoadingProjects) {
			unfiledChats.push(chat);
		}
	}
	return { chatsByProjectId, unfiledChats };
};
