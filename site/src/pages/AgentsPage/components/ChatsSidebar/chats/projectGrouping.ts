import type { Chat, ChatProject, Organization } from "#/api/typesGenerated";
import { canManageChat } from "../../ChatActionsMenuItems";

type ChatsGroupedByProject = {
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly unfiledChats: readonly Chat[];
	readonly folderProjectIds: ReadonlySet<string>;
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
	return {
		chatsByProjectId,
		unfiledChats,
		folderProjectIds: loadedProjectIds,
	};
};

type MergeProjectFolderChatsOptions = {
	readonly projectId: string;
	readonly filedChats: readonly Chat[];
	/** The folder's own fetch, which reaches chats the sidebar has not loaded. */
	readonly fetchedChats: readonly Chat[];
	readonly currentUserId: string;
	readonly archived: boolean;
};

/**
 * The filed copy wins because the sidebar list sees creates and moves first,
 * and fetched rows are checked again because cached rows change in place
 * after the fetch.
 */
export const mergeProjectFolderChats = ({
	projectId,
	filedChats,
	fetchedChats,
	currentUserId,
	archived,
}: MergeProjectFolderChatsOptions): readonly Chat[] => {
	const chatsById = new Map(filedChats.map((chat) => [chat.id, chat]));
	for (const chat of fetchedChats) {
		if (
			!chatsById.has(chat.id) &&
			chat.project_id === projectId &&
			chat.pin_order === 0 &&
			chat.archived === archived &&
			canManageChat(chat, currentUserId)
		) {
			chatsById.set(chat.id, chat);
		}
	}
	return [...chatsById.values()].sort(
		(a, b) =>
			Date.parse(b.updated_at) - Date.parse(a.updated_at) ||
			b.id.localeCompare(a.id),
	);
};

type CountProjectFolderPinsOptions = {
	readonly projectId: string;
	readonly filedPinnedChats: readonly Chat[];
	readonly fetchedChats: readonly Chat[];
	readonly currentUserId: string;
};

/** The folder's fetch adds pins that the sidebar's loaded pages lack. */
export const countProjectFolderPins = ({
	projectId,
	filedPinnedChats,
	fetchedChats,
	currentUserId,
}: CountProjectFolderPinsOptions): number => {
	const pinnedIds = new Set(filedPinnedChats.map((chat) => chat.id));
	for (const chat of fetchedChats) {
		if (
			chat.project_id === projectId &&
			chat.pin_order > 0 &&
			canManageChat(chat, currentUserId)
		) {
			pinnedIds.add(chat.id);
		}
	}
	return pinnedIds.size;
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
