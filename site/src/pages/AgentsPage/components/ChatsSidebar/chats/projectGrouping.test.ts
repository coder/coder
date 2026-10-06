import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { groupChatsByProject } from "./projectGrouping";

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
