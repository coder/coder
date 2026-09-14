import { describe, expect, it } from "vitest";
import { MockUserOwner, MockWorkspace } from "#/testHelpers/entities";
import { getWorkspaceOptionsWithLinkedWorkspace } from "./workspaceOptions";

describe("getWorkspaceOptionsWithLinkedWorkspace", () => {
	it("includes a missing linked workspace only when the current user owns it", () => {
		const existingWorkspace = {
			...MockWorkspace,
			id: "existing-workspace",
		};
		const ownerWorkspaceOptions = [existingWorkspace];
		const linkedWorkspace = {
			...MockWorkspace,
			id: "linked-workspace",
			owner_id: MockUserOwner.id,
		};

		expect(
			getWorkspaceOptionsWithLinkedWorkspace(
				ownerWorkspaceOptions,
				linkedWorkspace,
				MockUserOwner.id,
			),
		).toEqual([linkedWorkspace, existingWorkspace]);

		const sharedWorkspace = {
			...linkedWorkspace,
			owner_id: "another-user",
		};

		expect(
			getWorkspaceOptionsWithLinkedWorkspace(
				ownerWorkspaceOptions,
				sharedWorkspace,
				MockUserOwner.id,
			),
		).toBe(ownerWorkspaceOptions);
	});
});
