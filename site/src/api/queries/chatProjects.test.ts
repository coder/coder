import { MutationObserver, type QueryClient } from "react-query";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectRole, ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockChatProjectSummary,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	archiveChatProject,
	archivedChatProjectsKey,
	chatProject,
	chatProjectACLKey,
	chatProjectInstructionsKey,
	chatProjectMemberCountsKey,
	chatProjectSummariesKey,
	chatProjectsKey,
	createChatProject,
	deleteChatProject,
	joinChatProjectSummaries,
	pinChatProject,
	setChatProjectRole,
	unarchiveChatProject,
	unlistedChatProject,
	unpinChatProject,
	updateChatProject,
} from "./chatProjects";
import {
	chatEntitiesFamilyKey,
	chatListFamilyKey,
	chatPromptsKey,
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
		const nestedKey = chatPromptsKey("chat-1");
		queryClient.setQueryData(nestedKey, {});
		await settle(deleteChatProject(queryClient));
		for (const key of [...projectKeys, ...chatKeys]) {
			expect(isInvalidated(queryClient, key)).toBe(true);
		}
		// A chat's nested queries do not depend on its project.
		expect(isInvalidated(queryClient, nestedKey)).toBe(false);
	});

	it("moves a project between the active and archived lists and refreshes its chats", async () => {
		// Lists sort by lowercased name, so this sorts before "Launch".
		const mockOtherProject = {
			...MockChatProject,
			id: "other-project",
			name: "alpha",
		};
		const mockOtherSummary = {
			...MockChatProjectSummary,
			project_id: mockOtherProject.id,
		};
		vi.spyOn(API.experimental, "archiveChatProject").mockResolvedValue(
			MockArchivedChatProject,
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
				before: {
					active: [mockOtherProject, MockChatProject],
					archived: [],
					summaries: [MockChatProjectSummary, mockOtherSummary],
				},
				after: {
					active: [mockOtherProject],
					archived: [MockArchivedChatProject],
					summaries: [mockOtherSummary],
				},
			},
			{
				run: (queryClient: QueryClient) =>
					new MutationObserver(
						queryClient,
						unarchiveChatProject(queryClient),
					).mutate(MockArchivedChatProject),
				before: {
					active: [mockOtherProject],
					archived: [MockArchivedChatProject],
					summaries: [mockOtherSummary],
				},
				// Summaries pick the project up when they refetch.
				after: {
					active: [mockOtherProject, MockChatProject],
					archived: [],
					summaries: [mockOtherSummary],
				},
			},
		];
		for (const { run, before, after } of cases) {
			const queryClient = seed();
			const byWorkspaceKey = chatsByWorkspace(["workspace-1"]).queryKey;
			queryClient.setQueryData(byWorkspaceKey, {});
			queryClient.setQueryData(chatProjectsKey, before.active);
			queryClient.setQueryData(archivedChatProjectsKey, before.archived);
			queryClient.setQueryData(chatProjectSummariesKey, before.summaries);

			await run(queryClient);

			expect(queryClient.getQueryData(chatProjectsKey)).toEqual(after.active);
			expect(queryClient.getQueryData(archivedChatProjectsKey)).toEqual(
				after.archived,
			);
			expect(queryClient.getQueryData(chatProjectSummariesKey)).toEqual(
				after.summaries,
			);
			for (const key of [
				...chatKeys,
				byWorkspaceKey,
				chatProjectSummariesKey,
			]) {
				expect(isInvalidated(queryClient, key)).toBe(true);
			}
		}
		vi.restoreAllMocks();
	});

	it("updates a project found outside both lists without adding it to them", async () => {
		vi.spyOn(API.experimental, "getChatProject").mockResolvedValue(
			MockChatProject,
		);
		vi.spyOn(API.experimental, "archiveChatProject").mockResolvedValue(
			MockArchivedChatProject,
		);
		const queryClient = seed();
		const mockOtherProject = { ...MockChatProject, id: "other-project" };
		queryClient.setQueryData(chatProjectsKey, [mockOtherProject]);
		queryClient.setQueryData(archivedChatProjectsKey, []);
		const unlisted = unlistedChatProject(MockChatProject.id, [
			MockChatProject.organization_id,
		]);
		await queryClient.fetchQuery(unlisted);

		await new MutationObserver(
			queryClient,
			archiveChatProject(queryClient),
		).mutate({ project: MockChatProject, stopActiveChats: false });

		expect(queryClient.getQueryData(chatProjectsKey)).toEqual([
			mockOtherProject,
		]);
		expect(queryClient.getQueryData(archivedChatProjectsKey)).toEqual([]);
		expect(queryClient.getQueryData(unlisted.queryKey)).toEqual(
			MockArchivedChatProject,
		);
		vi.restoreAllMocks();
	});

	it("refreshes only the summaries after a pin change", async () => {
		for (const factory of [pinChatProject, unpinChatProject]) {
			const queryClient = seed();
			queryClient.setQueryData(chatProjectSummariesKey, []);
			await settle(factory(queryClient));
			expect(isInvalidated(queryClient, chatProjectSummariesKey)).toBe(true);
			expect(isInvalidated(queryClient, chatKeys[0])).toBe(false);
			expect(isInvalidated(queryClient, chatProjectsKey)).toBe(false);
			expect(isInvalidated(queryClient, chatKeys[1])).toBe(false);
		}
	});

	it("moves a pinned project in the cached order and restores it when the move fails", async () => {
		const mockBillingProject = { ...MockChatProject, id: "billing-project" };
		const mockBillingSummary = {
			...MockChatProjectSummary,
			project_id: mockBillingProject.id,
			pin_order: 2,
		};
		const queryClient = createTestQueryClient();
		// Keeps the summaries cached without an observer.
		queryClient.setQueryDefaults(chatProjectSummariesKey, {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatProjectSummariesKey, [
			MockChatProjectSummary,
			mockBillingSummary,
		]);
		const pinOrderById = () =>
			Object.fromEntries(
				(
					queryClient.getQueryData<ChatProjectSummary[]>(
						chatProjectSummariesKey,
					) ?? []
				).map((s) => [s.project_id, s.pin_order]),
			);
		let rejectPin: (error: unknown) => void = () => {};
		vi.spyOn(API.experimental, "pinChatProject").mockReturnValue(
			new Promise((_resolve, reject) => {
				rejectPin = reject;
			}),
		);
		onTestFinished(() => {
			vi.restoreAllMocks();
		});

		const move = new MutationObserver(
			queryClient,
			pinChatProject(queryClient),
		).mutate({ project: mockBillingProject, pinOrder: 1 });
		await vi.waitFor(() =>
			expect(pinOrderById()).toEqual({
				[mockBillingProject.id]: 1,
				[MockChatProject.id]: 2,
			}),
		);

		rejectPin(mockApiError({}));
		await expect(move).rejects.toBeDefined();
		expect(pinOrderById()).toEqual({
			[MockChatProject.id]: 1,
			[mockBillingProject.id]: 2,
		});
	});
});

describe("pinChatProject ordering", () => {
	const [mockLaunch, mockBilling, mockOps] = ["launch", "billing", "ops"].map(
		(id, index) => ({
			...MockChatProjectSummary,
			project_id: id,
			pin_order: index + 1,
		}),
	);
	const setup = () => {
		const queryClient = createTestQueryClient();
		// Keeps the summaries cached without an observer.
		queryClient.setQueryDefaults(chatProjectSummariesKey, {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatProjectSummariesKey, [
			mockLaunch,
			mockBilling,
			mockOps,
		]);
		const requests: { id: string; settle: (error?: unknown) => void }[] = [];
		const pin = vi.spyOn(API.experimental, "pinChatProject").mockImplementation(
			(_organizationId, id) =>
				new Promise((resolve, reject) => {
					requests.push({
						id,
						settle: (error) => (error ? reject(error) : resolve()),
					});
				}),
		);
		onTestFinished(() => {
			vi.restoreAllMocks();
		});
		const move = (summary: ChatProjectSummary, pinOrder: number) =>
			new MutationObserver(queryClient, pinChatProject(queryClient)).mutate({
				project: { ...MockChatProject, id: summary.project_id },
				pinOrder,
			});
		const pinnedIds = () =>
			(
				queryClient.getQueryData<ChatProjectSummary[]>(
					chatProjectSummariesKey,
				) ?? []
			)
				.toSorted((a, b) => a.pin_order - b.pin_order)
				.map((s) => s.project_id);
		return { queryClient, requests, pin, move, pinnedIds };
	};

	it("sends queued moves one at a time, in order", async () => {
		const { requests, pin, move } = setup();
		const first = move(mockOps, 1);
		const second = move(mockBilling, 1);
		await vi.waitFor(() => expect(requests).toHaveLength(1));
		expect(pin).toHaveBeenCalledTimes(1);
		expect(requests[0].id).toBe(mockOps.project_id);

		requests[0].settle();
		await first;
		await vi.waitFor(() => expect(requests).toHaveLength(2));
		expect(requests[1].id).toBe(mockBilling.project_id);
		requests[1].settle();
		await second;
	});

	it("keeps a later queued move when an earlier one fails", async () => {
		const { requests, move, pinnedIds } = setup();
		const first = move(mockOps, 1);
		const second = move(mockBilling, 1);
		await vi.waitFor(() =>
			expect(pinnedIds()).toEqual(["billing", "ops", "launch"]),
		);

		await vi.waitFor(() => expect(requests.length).toBeGreaterThan(0));
		requests[0].settle(mockApiError({}));
		await expect(first).rejects.toBeDefined();
		expect(pinnedIds()).toEqual(["billing", "ops", "launch"]);
		await vi.waitFor(() => expect(requests).toHaveLength(2));
		requests[1].settle();
		await second;
	});
});

describe("joinChatProjectSummaries", () => {
	it("pairs summaries with their projects in summary order and leaves out a summary whose project is not listed", () => {
		const mockOtherProject = { ...MockChatProject, id: "other-project" };

		const joined = joinChatProjectSummaries(
			[MockChatProject, mockOtherProject],
			[
				{ ...MockChatProjectSummary, project_id: mockOtherProject.id },
				{ ...MockChatProjectSummary, project_id: "unlisted-project" },
				MockChatProjectSummary,
			],
		);

		expect(joined.map((summary) => summary.project)).toEqual([
			mockOtherProject,
			MockChatProject,
		]);
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
		chatProjectMemberCountsKey,
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
