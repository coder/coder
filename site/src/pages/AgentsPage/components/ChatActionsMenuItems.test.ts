import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectAgentUser,
	MockUserOwner,
} from "#/testHelpers/entities";
import { canDriveChat, canManageChat } from "./ChatActionsMenuItems";

const sharedByAnotherUser = {
	...MockChat,
	owner_id: "sharing-user",
	shared: true,
};

describe("canManageChat", () => {
	it("is true only for the chat owner", () => {
		expect(canManageChat(MockChat, MockUserOwner.id)).toBe(true);
		expect(canManageChat(sharedByAnotherUser, MockUserOwner.id)).toBe(false);
	});
});

describe("canDriveChat", () => {
	const agentChat = {
		...MockChat,
		owner_id: MockChatProjectAgentUser.id,
		project_id: MockChatProject.id,
	};

	it("lets the owner drive their own chat", () => {
		expect(canDriveChat(MockChat, MockUserOwner.id, [])).toBe(true);
	});

	it("lets someone who can use the agent drive its agent user's chats", () => {
		expect(canDriveChat(agentChat, "someone", [MockChatProject])).toBe(true);
	});

	it("does not when the agent is not in the viewer's list", () => {
		expect(canDriveChat(agentChat, "someone", [])).toBe(false);
	});

	it("does not when the chat is owned by someone other than the agent user", () => {
		expect(
			canDriveChat({ ...agentChat, owner_id: "other-user" }, "someone", [
				MockChatProject,
			]),
		).toBe(false);
	});
});
