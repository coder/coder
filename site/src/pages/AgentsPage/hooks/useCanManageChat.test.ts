import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { canManageChat, chatUpdateChecks } from "./useCanManageChat";

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

describe("chatUpdateChecks", () => {
	it("asks for update on every chat in each organization", () => {
		expect(chatUpdateChecks(["org-a", "org-b"])).toEqual({
			"org-a": {
				object: { resource_type: "chat", organization_id: "org-a" },
				action: "update",
			},
			"org-b": {
				object: { resource_type: "chat", organization_id: "org-b" },
				action: "update",
			},
		});
	});
});
