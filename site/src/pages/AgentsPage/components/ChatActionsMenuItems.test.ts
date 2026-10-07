import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { canManageChat } from "./ChatActionsMenuItems";

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
