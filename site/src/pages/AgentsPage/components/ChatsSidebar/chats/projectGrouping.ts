import type { Chat, ChatProject, Organization } from "#/api/typesGenerated";

type ChatsGroupedByProject = {
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly unfiledChats: readonly Chat[];
};

/**
 * Splits chats into their project folders. While projects are loading, chats
 * with a project are held back so they do not jump from the unfiled list into
 * a folder. Once loaded, a chat whose project is not in `projects`, for
 * example because the request failed, stays unfiled so it never disappears
 * from the sidebar.
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

/**
 * Maps project IDs to their organization's name for projects whose name is
 * also used in another organization, so same-named projects stay
 * distinguishable.
 */
export const getOrganizationLabels = (
	projects: readonly ChatProject[],
	organizations: readonly Organization[],
): ReadonlyMap<string, string> => {
	const organizationIdsByName = new Map<string, Set<string>>();
	for (const project of projects) {
		const name = project.name.toLowerCase();
		const organizationIds = organizationIdsByName.get(name) ?? new Set();
		organizationIds.add(project.organization_id);
		organizationIdsByName.set(name, organizationIds);
	}

	const labels = new Map<string, string>();
	for (const project of projects) {
		const name = project.name.toLowerCase();
		if ((organizationIdsByName.get(name)?.size ?? 0) < 2) {
			continue;
		}
		const organization = organizations.find(
			(org) => org.id === project.organization_id,
		);
		if (organization) {
			labels.set(project.id, organization.display_name || organization.name);
		}
	}
	return labels;
};
