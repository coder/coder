import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	setChatProjectGroupRole,
	setChatProjectUserRole,
} from "./chatProjects";
import { chatProjectACLKey, chatProjectsKey } from "./chatProjectsKeys";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			updateChatProjectACL: vi.fn(),
		},
	},
}));

describe("chat project sharing mutations", () => {
	it("sets one user role and invalidates the ACL and project list", async () => {
		const queryClient = createTestQueryClient();
		const projectId = "project-1";
		queryClient.setQueryData(chatProjectACLKey(projectId), {
			users: [],
			groups: [],
		});
		queryClient.setQueryData(chatProjectsKey("org-1"), []);
		vi.mocked(API.experimental.updateChatProjectACL).mockResolvedValue();

		const mutation = setChatProjectUserRole(queryClient);
		const variables = { projectId, userId: "user-1", role: "read" as const };
		await mutation.mutationFn(variables);
		expect(API.experimental.updateChatProjectACL).toHaveBeenCalledWith(
			projectId,
			{ user_roles: { "user-1": "read" } },
		);

		await mutation.onSuccess(undefined, variables);
		expect(
			queryClient.getQueryState(chatProjectACLKey(projectId))?.isInvalidated,
		).toBe(true);
		expect(
			queryClient.getQueryState(chatProjectsKey("org-1"))?.isInvalidated,
		).toBe(true);
	});

	it("removes a group role through the group ACL", async () => {
		const queryClient = createTestQueryClient();
		const projectId = "project-1";
		vi.mocked(API.experimental.updateChatProjectACL).mockResolvedValue();

		const mutation = setChatProjectGroupRole(queryClient);
		await mutation.mutationFn({ projectId, groupId: "group-1", role: "" });
		expect(API.experimental.updateChatProjectACL).toHaveBeenCalledWith(
			projectId,
			{ group_roles: { "group-1": "" } },
		);
	});
});
