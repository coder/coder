import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	getOrganizationLabels,
	groupChatsByProject,
	mergeProjectFolderChats,
} from "./projectGrouping";

describe("groupChatsByProject", () => {
	it("files chats under a loaded project and leaves the rest unfiled", () => {
		const mockLooseChat = { ...MockChat, id: "loose" };
		const mockFiledChat = {
			...MockChat,
			id: "filed",
			organization_id: MockChatProject.organization_id,
			project_id: MockChatProject.id,
		};
		const mockUnloadedProjectChat = {
			...MockChat,
			id: "unloaded",
			project_id: "unloaded-project",
		};

		const grouped = groupChatsByProject(
			[mockLooseChat, mockFiledChat, mockUnloadedProjectChat],
			[MockChatProject],
		);

		expect(grouped.chatsByProjectId.get(MockChatProject.id)).toEqual([
			mockFiledChat,
		]);
		expect(grouped.unfiledChats).toEqual([
			mockLooseChat,
			mockUnloadedProjectChat,
		]);
	});

	it("leaves every chat unfiled when no projects are loaded", () => {
		const mockFiledChat = {
			...MockChat,
			id: "filed",
			organization_id: MockChatProject.organization_id,
			project_id: MockChatProject.id,
		};

		const grouped = groupChatsByProject([mockFiledChat], []);

		expect(grouped.chatsByProjectId.size).toBe(0);
		expect(grouped.unfiledChats).toEqual([mockFiledChat]);
	});
});

describe("mergeProjectFolderChats", () => {
	const mockFolderChat: Chat = {
		...MockChat,
		id: "folder-chat",
		title: "Folder chat",
		owner_id: MockUserOwner.id,
		project_id: MockChatProject.id,
		pin_order: 0,
		archived: false,
		updated_at: "2026-10-02T00:00:00Z",
	};
	const mockOlderChat: Chat = {
		...mockFolderChat,
		id: "older-chat",
		title: "Older chat",
		updated_at: "2026-10-01T00:00:00Z",
	};
	const mockNewerChat: Chat = {
		...mockFolderChat,
		id: "newer-chat",
		title: "Newer chat",
		updated_at: "2026-10-03T00:00:00Z",
	};
	const merge = (filedChats: Chat[], fetchedChats: Chat[]) =>
		mergeProjectFolderChats({
			projectId: MockChatProject.id,
			filedChats,
			fetchedChats,
			currentUserId: MockUserOwner.id,
			archived: false,
		}).map((chat) => chat.title);

	it("adds fetched chats the sidebar has not loaded, newest first", () => {
		expect(merge([mockFolderChat], [mockOlderChat, mockNewerChat])).toEqual([
			"Newer chat",
			"Folder chat",
			"Older chat",
		]);
	});

	it("keeps the filed copy of a chat that both lists hold", () => {
		const mockStaleFetchedChat = { ...mockFolderChat, title: "Stale title" };

		expect(merge([mockFolderChat], [mockStaleFetchedChat])).toEqual([
			"Folder chat",
		]);
	});

	it("drops fetched chats that no longer belong in the folder", () => {
		const mockPinnedChat = { ...mockFolderChat, pin_order: 1 };
		const mockMovedChat = { ...mockFolderChat, project_id: "other-project" };
		const mockArchivedChat = { ...mockFolderChat, archived: true };
		const mockSharedChat = { ...mockFolderChat, owner_id: MockUserMember.id };

		expect(
			merge(
				[],
				[mockPinnedChat, mockMovedChat, mockArchivedChat, mockSharedChat],
			),
		).toEqual([]);
	});
});

describe("getOrganizationLabels", () => {
	const organizations = [MockDefaultOrganization, MockOrganization2];

	it("labels projects whose name is used in another organization", () => {
		const mockOtherProject = {
			...MockChatProject,
			id: "other",
			name: MockChatProject.name.toUpperCase(),
			organization_id: MockOrganization2.id,
		};

		const labels = getOrganizationLabels(
			[MockChatProject, mockOtherProject],
			organizations,
		);

		expect(labels.get(MockChatProject.id)).toBe(
			MockDefaultOrganization.display_name,
		);
		expect(labels.get(mockOtherProject.id)).toBe(
			MockOrganization2.display_name,
		);
	});

	it("leaves unique names and same-organization duplicates unlabeled", () => {
		const mockSameOrganizationProject = { ...MockChatProject, id: "same" };
		const mockUniqueProject = {
			...MockChatProject,
			id: "unique",
			name: "Unique",
			organization_id: MockOrganization2.id,
		};

		const labels = getOrganizationLabels(
			[MockChatProject, mockSameOrganizationProject, mockUniqueProject],
			organizations,
		);

		expect(labels.size).toBe(0);
	});
});
