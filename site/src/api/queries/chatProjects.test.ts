import { MutationObserver } from "react-query";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectRole, ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	chatProject,
	chatProjectACLKey,
	chatProjectInstructionsKey,
	chatProjectSummariesKey,
	chatProjectsKey,
	createChatProject,
	deleteChatProject,
	setChatProjectRole,
	unpinChatProject,
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

	it("refreshes the summaries and chat lists, not searches, after an unpin", async () => {
		const queryClient = seed();
		queryClient.setQueryData(chatProjectSummariesKey, []);
		await settle(unpinChatProject(queryClient));
		expect(isInvalidated(queryClient, chatProjectSummariesKey)).toBe(true);
		expect(isInvalidated(queryClient, chatKeys[0])).toBe(true);
		expect(isInvalidated(queryClient, chatProjectsKey)).toBe(false);
		expect(isInvalidated(queryClient, chatKeys[1])).toBe(false);
	});
});

describe("unpinChatProject", () => {
	it("unpins the project in the cached summaries at once and restores it when the unpin fails", async () => {
		const queryClient = createTestQueryClient();
		// Keeps the summaries cached without an observer.
		queryClient.setQueryDefaults(chatProjectSummariesKey, {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatProjectSummariesKey, [MockChatProjectSummary]);
		const isPinned = () =>
			queryClient.getQueryData<ChatProjectSummary[]>(
				chatProjectSummariesKey,
			)?.[0].pinned;
		let rejectUnpin: (error: unknown) => void = () => {};
		vi.spyOn(API.experimental, "unpinChatProject").mockReturnValue(
			new Promise((_resolve, reject) => {
				rejectUnpin = reject;
			}),
		);
		onTestFinished(() => {
			vi.restoreAllMocks();
		});

		const unpin = new MutationObserver(
			queryClient,
			unpinChatProject(queryClient),
		).mutate(MockChatProject);
		await vi.waitFor(() => expect(isPinned()).toBe(false));

		rejectUnpin(mockApiError({}));
		await expect(unpin).rejects.toBeDefined();
		expect(isPinned()).toBe(true);
	});
});

type Principal = { kind: "user" | "group"; id: string };

describe("setChatProjectRole", () => {
	const refreshedKeys = [
		chatProjectsKey,
		chatProjectACLKey(MockChatProject.id),
		// Summaries carry the viewer's permissions on the project.
		chatProjectSummariesKey,
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
