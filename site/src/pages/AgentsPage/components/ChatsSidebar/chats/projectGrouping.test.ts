import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { groupChatsByProject } from "./projectGrouping";

describe("groupChatsByProject", () => {
	it("files chats under a loaded project and leaves the rest unfiled", () => {
		const loose = { ...MockChat, id: "loose" };
		const filed = { ...MockChat, id: "filed", project_id: MockChatProject.id };
		const unloaded = {
			...MockChat,
			id: "unloaded",
			project_id: "project-in-another-organization",
		};

		const grouped = groupChatsByProject(
			[loose, filed, unloaded],
			[MockChatProject],
		);

		expect(grouped.chatsByProjectId.get(MockChatProject.id)).toEqual([filed]);
		expect(grouped.unfiledChats).toEqual([loose, unloaded]);
	});

	it("leaves every chat unfiled when no projects are loaded", () => {
		const filed = { ...MockChat, id: "filed", project_id: MockChatProject.id };

		const grouped = groupChatsByProject([filed], []);

		expect(grouped.chatsByProjectId.size).toBe(0);
		expect(grouped.unfiledChats).toEqual([filed]);
	});
});
