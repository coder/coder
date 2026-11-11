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
	pinChatProject,
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

	it("refreshes the summaries and chat lists, not searches, after a pin change", async () => {
		for (const factory of [pinChatProject, unpinChatProject]) {
			const queryClient = seed();
			queryClient.setQueryData(chatProjectSummariesKey, []);
			await settle(factory(queryClient));
			expect(isInvalidated(queryClient, chatProjectSummariesKey)).toBe(true);
			expect(isInvalidated(queryClient, chatKeys[0])).toBe(true);
			expect(isInvalidated(queryClient, chatProjectsKey)).toBe(false);
			expect(isInvalidated(queryClient, chatKeys[1])).toBe(false);
		}
	});

	it("moves a pinned project in the cached order and restores it when the move fails", async () => {
		const mockBillingSummary = {
			...MockChatProjectSummary,
			project: { ...MockChatProject, id: "billing-project" },
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
				).map((s) => [s.project.id, s.pin_order]),
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
		).mutate({ project: mockBillingSummary.project, pinOrder: 1 });
		await vi.waitFor(() =>
			expect(pinOrderById()).toEqual({
				[mockBillingSummary.project.id]: 1,
				[MockChatProject.id]: 2,
			}),
		);

		rejectPin(mockApiError({}));
		await expect(move).rejects.toBeDefined();
		expect(pinOrderById()).toEqual({
			[MockChatProject.id]: 1,
			[mockBillingSummary.project.id]: 2,
		});
	});
});

describe("pinChatProject ordering", () => {
	// Launch, Billing and Ops are pinned in that order.
	const [launch, billing, ops] = ["launch", "billing", "ops"].map(
		(id, index) => ({
			...MockChatProjectSummary,
			project: { ...MockChatProject, id },
			pin_order: index + 1,
		}),
	);
	const setup = () => {
		const queryClient = createTestQueryClient();
		// Keeps the queries cached without an observer.
		for (const key of [chatProjectSummariesKey, chatListFamilyKey]) {
			queryClient.setQueryDefaults(key, { gcTime: Number.POSITIVE_INFINITY });
		}
		queryClient.setQueryData(chatProjectSummariesKey, [launch, billing, ops]);
		const chatListKey = [...chatListFamilyKey, { q: "" }];
		queryClient.setQueryData(chatListKey, {});
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
				project: summary.project,
				pinOrder,
			});
		const pinnedIds = () =>
			(
				queryClient.getQueryData<ChatProjectSummary[]>(
					chatProjectSummariesKey,
				) ?? []
			)
				.toSorted((a, b) => a.pin_order - b.pin_order)
				.map((s) => s.project.id);
		return { queryClient, chatListKey, requests, pin, move, pinnedIds };
	};

	it("sends queued moves one at a time, in order", async () => {
		const { requests, pin, move } = setup();
		const first = move(ops, 1);
		const second = move(billing, 1);
		await vi.waitFor(() => expect(requests).toHaveLength(1));
		expect(pin).toHaveBeenCalledTimes(1);
		expect(requests[0].id).toBe(ops.project.id);

		requests[0].settle();
		await first;
		await vi.waitFor(() => expect(requests).toHaveLength(2));
		expect(requests[1].id).toBe(billing.project.id);
		requests[1].settle();
		await second;
	});

	it("keeps a later queued move when an earlier one fails", async () => {
		const { requests, move, pinnedIds } = setup();
		const first = move(ops, 1);
		const second = move(billing, 1);
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

	it("refreshes only the summaries after a move, since no chat changes lists", async () => {
		const { queryClient, chatListKey, requests, move } = setup();
		const moved = move(ops, 1);
		await vi.waitFor(() => expect(requests).toHaveLength(1));
		requests[0].settle();
		await moved;
		expect(
			queryClient.getQueryState(chatProjectSummariesKey)?.isInvalidated,
		).toBe(true);
		expect(queryClient.getQueryState(chatListKey)?.isInvalidated).toBe(false);
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
