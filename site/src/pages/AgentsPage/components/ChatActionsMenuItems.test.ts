import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { canManageChat, chatHasMenuActions } from "./ChatActionsMenuItems";

const sharedByAnotherUser = {
	...MockChat,
	owner_id: "sharing-user",
	shared: true,
};

describe("canManageChat", () => {
	it("is true for the chat owner before permissions load", () => {
		expect(canManageChat(MockChat, MockUserOwner.id, undefined)).toBe(true);
		expect(
			canManageChat(sharedByAnotherUser, MockUserOwner.id, undefined),
		).toBe(false);
	});

	it("follows the organization-wide chat:update permission for other users' chats", () => {
		const canUpdate = { [MockChat.organization_id]: true };
		const cannotUpdate = { [MockChat.organization_id]: false };
		expect(
			canManageChat(sharedByAnotherUser, MockUserOwner.id, canUpdate),
		).toBe(true);
		expect(
			canManageChat(sharedByAnotherUser, MockUserOwner.id, cannotUpdate),
		).toBe(false);
		expect(
			canManageChat(
				{ ...sharedByAnotherUser, organization_id: "other-org" },
				MockUserOwner.id,
				canUpdate,
			),
		).toBe(false);
	});
});

describe("chatHasMenuActions", () => {
	it("hides the menu from viewers unless the subagents toggle is available", () => {
		expect(chatHasMenuActions(sharedByAnotherUser, { canManage: false })).toBe(
			false,
		);
		expect(
			chatHasMenuActions(sharedByAnotherUser, {
				canManage: false,
				hasSubagentsToggle: true,
			}),
		).toBe(true);
	});

	it("hides the menu from owners only for archived child chats", () => {
		expect(chatHasMenuActions(MockChat, { canManage: true })).toBe(true);
		expect(
			chatHasMenuActions({ ...MockChat, archived: true }, { canManage: true }),
		).toBe(true);
		expect(
			chatHasMenuActions(
				{ ...MockChat, archived: true, parent_chat_id: "parent-chat" },
				{ canManage: true },
			),
		).toBe(false);
	});
});
