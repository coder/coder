import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { getOrganizationLabels, groupChatsByProject } from "./projectGrouping";

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
