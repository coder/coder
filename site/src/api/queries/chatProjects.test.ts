import { MutationObserver } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	chatProject,
	chatProjectACLKey,
	chatProjectInstructionsKey,
	chatProjectsKey,
	createChatProject,
	deleteChatProject,
	setChatProjectGroupRole,
	setChatProjectUserRole,
	updateChatProject,
} from "./chatProjects";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	chatMessagesKey,
	chatSearchFamilyKey,
} from "./chats";

describe("chatProject", () => {
	it("selects the project with the ID, or null when the list lacks it", () => {
		const other = { ...MockChatProject, id: "other-project" };
		const projects = [MockChatProject, other];

		expect(chatProject(other.id).select?.(projects)).toBe(other);
		expect(chatProject("missing").select?.(projects)).toBeNull();
	});
});

describe("chat project mutations", () => {
	const projectKeys = [chatProjectsKey];
	const chatKeys = [
		[...chatListFamilyKey, { q: "" }],
		[...chatSearchFamilyKey, { q: "x" }],
		[...chatEntitiesFamilyKey, "chat-1"],
	];
	const seed = () => {
		const queryClient = createTestQueryClient();
		for (const key of [...projectKeys, ...chatKeys]) {
			queryClient.setQueryData(key, {});
		}
		return queryClient;
	};
	// The callbacks ignore their arguments.
	const settle = (options: { onSettled?: (...args: never[]) => unknown }) =>
		options.onSettled?.();
	const isInvalidated = (
		queryClient: ReturnType<typeof createTestQueryClient>,
		key: readonly unknown[],
	) => queryClient.getQueryState(key)?.isInvalidated;

	it("refreshes only the project list after a create or update", async () => {
		for (const factory of [createChatProject, updateChatProject]) {
			const queryClient = seed();
			await settle(factory(queryClient));
			expect(isInvalidated(queryClient, chatProjectsKey)).toBe(true);
			for (const key of chatKeys) {
				expect(isInvalidated(queryClient, key)).toBe(false);
			}
		}
	});

	it("refreshes chat lists, searches, and entities after a delete", async () => {
		const queryClient = seed();
		const messagesKey = chatMessagesKey("chat-1");
		queryClient.setQueryData(messagesKey, {});
		await settle(deleteChatProject(queryClient));
		for (const key of [...projectKeys, ...chatKeys]) {
			expect(isInvalidated(queryClient, key)).toBe(true);
		}
		// A chat's nested queries do not depend on its project.
		expect(isInvalidated(queryClient, messagesKey)).toBe(false);
	});
});

describe("chat project access mutations", () => {
	const aclKey = chatProjectACLKey(MockChatProject.id);
	const otherKeys = [
		chatProjectsKey,
		chatProjectInstructionsKey(MockChatProject.id),
		chatProjectACLKey("other-project"),
	];
	const setup = () => {
		const queryClient = createTestQueryClient();
		for (const key of [aclKey, ...otherKeys]) {
			queryClient.setQueryData(key, {});
		}
		const updateACL = vi
			.spyOn(API.experimental, "updateChatProjectACL")
			.mockResolvedValue();
		return { queryClient, updateACL };
	};
	const expectOnlyACLInvalidated = (
		queryClient: ReturnType<typeof createTestQueryClient>,
	) => {
		expect(queryClient.getQueryState(aclKey)?.isInvalidated).toBe(true);
		for (const key of otherKeys) {
			expect(queryClient.getQueryState(key)?.isInvalidated).toBe(false);
		}
	};

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("sets a user's role and refreshes only the project's access", async () => {
		const { queryClient, updateACL } = setup();

		await new MutationObserver(
			queryClient,
			setChatProjectUserRole(queryClient),
		).mutate({ project: MockChatProject, userId: "user-1", role: "admin" });

		expect(updateACL).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ user_roles: { "user-1": "admin" } },
		);
		expectOnlyACLInvalidated(queryClient);
	});

	it("removes a group and refreshes only the project's access", async () => {
		const { queryClient, updateACL } = setup();

		await new MutationObserver(
			queryClient,
			setChatProjectGroupRole(queryClient),
		).mutate({ project: MockChatProject, groupId: "group-1", role: "" });

		expect(updateACL).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ group_roles: { "group-1": "" } },
		);
		expectOnlyACLInvalidated(queryClient);
	});
});
