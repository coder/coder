import type {
	Chat,
	ChatProject,
	ChatProjectSummary,
	Organization,
} from "#/api/typesGenerated";
import { isOwnPinnedChat } from "../../../utils/pinnedChats";
import { canManageChat } from "../../ChatActionsMenuItems";

type ChatsGroupedByProject = {
	readonly chatsByProjectId: ReadonlyMap<string, readonly Chat[]>;
	readonly unfiledChats: readonly Chat[];
	readonly folderProjectIds: ReadonlySet<string>;
};

/**
 * Splits chats into the folders of the user's pinned projects. Chats with a
 * project are held back while the summaries load, and so are the user's own
 * chats of a project the summaries list as unpinned, which the chat list
 * keeps until it refetches. Chats shared with the user are listed whether or
 * not their project is pinned, so outside a folder they stay unfiled. A chat
 * whose project the summaries lack stays unfiled so it never leaves the
 * sidebar.
 */
export const groupChatsByProject = (
	chats: readonly Chat[],
	summaries: readonly ChatProjectSummary[],
	currentUserId: string,
	isLoadingProjects = false,
): ChatsGroupedByProject => {
	const folderProjectIds = new Set(
		summaries.filter((s) => s.pinned).map((s) => s.project.id),
	);
	const unpinnedProjectIds = new Set(
		summaries.filter((s) => !s.pinned).map((s) => s.project.id),
	);
	const chatsByProjectId = new Map<string, Chat[]>();
	const unfiledChats: Chat[] = [];
	for (const chat of chats) {
		if (chat.project_id && folderProjectIds.has(chat.project_id)) {
			const bucket = chatsByProjectId.get(chat.project_id);
			if (bucket) {
				bucket.push(chat);
			} else {
				chatsByProjectId.set(chat.project_id, [chat]);
			}
			continue;
		}
		if (
			chat.project_id &&
			(isLoadingProjects ||
				(canManageChat(chat, currentUserId) &&
					unpinnedProjectIds.has(chat.project_id)))
		) {
			continue;
		}
		unfiledChats.push(chat);
	}
	return { chatsByProjectId, unfiledChats, folderProjectIds };
};

type MergeProjectFolderChatsOptions = {
	readonly projectId: string;
	readonly filedChats: readonly Chat[];
	/** The folder's own fetch, which reaches chats the sidebar has not loaded. */
	readonly fetchedChats: readonly Chat[];
	/** Every chat the sidebar has loaded, whatever section it sits in. */
	readonly sidebarChatById: ReadonlyMap<string, Chat>;
	readonly currentUserId: string;
	readonly archived: boolean;
};

/**
 * A fetched row is used only for chats the sidebar has not loaded: the
 * sidebar sees creates, moves, and pins first, and a collapsed folder's
 * cached copy can be stale. Fetched rows are checked again because cached
 * rows change in place after the fetch. The user's pinned chats are listed
 * under Pinned instead.
 */
export const mergeProjectFolderChats = ({
	projectId,
	filedChats,
	fetchedChats,
	sidebarChatById,
	currentUserId,
	archived,
}: MergeProjectFolderChatsOptions): readonly Chat[] => {
	const chatsById = new Map(filedChats.map((chat) => [chat.id, chat]));
	for (const chat of fetchedChats) {
		if (
			!sidebarChatById.has(chat.id) &&
			chat.project_id === projectId &&
			chat.archived === archived &&
			!isOwnPinnedChat(chat, currentUserId)
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
	readonly sidebarChatById: ReadonlyMap<string, Chat>;
	readonly currentUserId: string;
};

/** The folder's fetch adds pins that the sidebar's loaded pages lack. */
export const countProjectFolderPins = ({
	projectId,
	filedPinnedChats,
	fetchedChats,
	sidebarChatById,
	currentUserId,
}: CountProjectFolderPinsOptions): number => {
	const pinnedIds = new Set(filedPinnedChats.map((chat) => chat.id));
	for (const chat of fetchedChats) {
		if (
			!sidebarChatById.has(chat.id) &&
			chat.project_id === projectId &&
			isOwnPinnedChat(chat, currentUserId)
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
