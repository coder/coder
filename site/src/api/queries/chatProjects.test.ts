import { MutationObserver, type QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectRole } from "#/api/typesGenerated";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	archiveChatProject,
	archivedChatProjectsKey,
	chatProject,
	chatProjectACLKey,
	chatProjectInstructionsKey,
	chatProjectsKey,
	createChatProject,
	deleteChatProject,
	setChatProjectRole,
	unarchiveChatProject,
	updateChatProject,
} from "./chatProjects";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	chatMessagesKey,
	chatSearchFamilyKey,
	chatsByWorkspace,
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

	it("moves a project between the active and archived lists and refreshes its chats", async () => {
		const other = { ...MockChatProject, id: "other-project" };
		const archived = {
			...MockChatProject,
			archived_at: "2026-11-10T00:00:00Z",
		};
		vi.spyOn(API.experimental, "archiveChatProject").mockResolvedValue(
			archived,
		);
		vi.spyOn(API.experimental, "unarchiveChatProject").mockResolvedValue(
			MockChatProject,
		);
		const cases = [
			{
				run: (queryClient: QueryClient) =>
					new MutationObserver(
						queryClient,
						archiveChatProject(queryClient),
					).mutate({ project: MockChatProject, stopActiveChats: false }),
				before: { active: [MockChatProject, other], archived: [] },
				after: { active: [other], archived: [archived] },
			},
			{
				run: (queryClient: QueryClient) =>
					new MutationObserver(
						queryClient,
						unarchiveChatProject(queryClient),
					).mutate(archived),
				before: { active: [other], archived: [archived] },
				after: { active: [MockChatProject, other], archived: [] },
			},
		];
		for (const { run, before, after } of cases) {
			const queryClient = seed();
			const byWorkspaceKey = chatsByWorkspace(["workspace-1"]).queryKey;
			queryClient.setQueryData(byWorkspaceKey, {});
			queryClient.setQueryData(chatProjectsKey, before.active);
			queryClient.setQueryData(archivedChatProjectsKey, before.archived);

			await run(queryClient);

			expect(queryClient.getQueryData(chatProjectsKey)).toEqual(after.active);
			expect(queryClient.getQueryData(archivedChatProjectsKey)).toEqual(
				after.archived,
			);
			for (const key of [...chatKeys, byWorkspaceKey]) {
				expect(isInvalidated(queryClient, key)).toBe(true);
			}
		}
		vi.restoreAllMocks();
	});
});

type Principal = { kind: "user" | "group"; id: string };

describe("setChatProjectRole", () => {
	const refreshedKeys = [
		chatProjectsKey,
		chatProjectACLKey(MockChatProject.id),
	];
	const otherKeys = [
		chatProjectInstructionsKey(MockChatProject.id),
		chatProjectACLKey("other-project"),
	];
	const setup = () => {
		const queryClient = createTestQueryClient();
		for (const key of [...refreshedKeys, ...otherKeys]) {
			queryClient.setQueryData(key, {});
		}
		const updateACL = vi
			.spyOn(API.experimental, "updateChatProjectACL")
			.mockResolvedValue();
		const mutate = (principal: Principal, role: ChatProjectRole) =>
			new MutationObserver(queryClient, setChatProjectRole(queryClient)).mutate(
				{ project: MockChatProject, principal, role },
			);
		return { queryClient, updateACL, mutate };
	};

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("sends user and group role changes", async () => {
		const { updateACL, mutate } = setup();

		await mutate({ kind: "user", id: "user-1" }, "admin");
		await mutate({ kind: "group", id: "group-1" }, "");

		const { organization_id, id } = MockChatProject;
		expect(updateACL.mock.calls).toEqual([
			[organization_id, id, { user_roles: { "user-1": "admin" } }],
			[organization_id, id, { group_roles: { "group-1": "" } }],
		]);
	});

	it("refreshes the project and its access after a change, even a forbidden one", async () => {
		for (const forbidden of [false, true]) {
			const { queryClient, updateACL, mutate } = setup();
			if (forbidden) {
				updateACL.mockRejectedValue({ ...mockApiError({}), status: 403 });
			}

			await mutate({ kind: "user", id: "user-1" }, "use").catch((error) =>
				expect(error).toMatchObject({ status: 403 }),
			);

			for (const key of refreshedKeys) {
				expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
			}
			for (const key of otherKeys) {
				expect(queryClient.getQueryState(key)?.isInvalidated).toBe(false);
			}
		}
	});
});
