import { describe, expect, it } from "vitest";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { canToggleManageAutomations } from "./manageAutomations";

describe("canToggleManageAutomations", () => {
	it.each([
		{
			name: "the owner of a root chat with the experiment on",
			chat: MockChat,
			viewerId: MockUserOwner.id,
			automationsExperimentEnabled: true,
			expected: true,
		},
		{
			name: "the experiment off",
			chat: MockChat,
			viewerId: MockUserOwner.id,
			automationsExperimentEnabled: false,
			expected: false,
		},
		{
			name: "a viewer who is not the owner",
			chat: MockChat,
			viewerId: "viewer-2",
			automationsExperimentEnabled: true,
			expected: false,
		},
		{
			name: "a sub-agent chat",
			chat: { ...MockChat, parent_chat_id: "parent-1" },
			viewerId: MockUserOwner.id,
			automationsExperimentEnabled: true,
			expected: false,
		},
	])("returns $expected for $name", ({ expected, ...input }) => {
		expect(canToggleManageAutomations(input)).toBe(expected);
	});
});
