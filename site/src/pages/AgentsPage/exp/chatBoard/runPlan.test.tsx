import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import {
	QueryClient,
	QueryClientProvider,
	useMutation,
	useQueryClient,
} from "react-query";
import { toast } from "sonner";
import { afterEach, describe, expect, it, type MockInstance, vi } from "vitest";
import { API } from "#/api/api";
import { infiniteChats, updateChatTitle } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred, type Deferred } from "#/testHelpers/deferred";
import {
	addColumn,
	type BoardState,
	mergeCards,
	moveCard,
	moveNote,
	renameCard,
} from "./boardApi";
import { boardChatsKey, boardWriteScope, updateChatLabels } from "./boardChats";
import { addCommentLabels, buildCards, buildColumns } from "./boardLabels";
import type { BoardStorage } from "./boardStorage";
import { type PlanDeps, runPlan } from "./runPlan";

vi.mock("sonner", () => {
	const toast = Object.assign(vi.fn(), {
		error: vi.fn(),
		success: vi.fn(),
		warning: vi.fn(),
	});
	return { toast };
});

/** The Undo action of the most recent confirmation toast. */
const lastUndo = () => {
	const [, options] = vi.mocked(toast).mock.lastCall ?? [];
	const action = (options as { action?: { onClick: () => void } })?.action;
	if (!action) throw new Error("no undo action on the last toast");
	return action.onClick;
};

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const stateOf = (
	chats: readonly Chat[],
	storage: Partial<BoardStorage> = {},
): BoardState => {
	const cards = buildCards(chats);
	const full: BoardStorage = {
		columnOrder: [],
		emptyColumns: [],
		windows: [],
		...storage,
	};
	return {
		cards,
		columns: buildColumns(cards, full.columnOrder, full.emptyColumns),
		storage: full,
	};
};

const renderDeps = () => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const updateStorage = vi.fn();
	// The page wires the two mutations the same way; the test runs them for
	// real so the cache behaviour of updateChatLabels is covered.
	const deps = renderHook(
		(): PlanDeps => {
			const queryClient = useQueryClient();
			const labels = useMutation({
				...updateChatLabels(queryClient),
				onError: (error: Error) => toast.error(error.message),
			});
			const titles = useMutation({
				...updateChatTitle(queryClient),
				scope: boardWriteScope,
			});
			return {
				write: (chatId, map, after) =>
					labels.mutateAsync({ chatId, labels: map, after }),
				rename: (chatId, title) => titles.mutateAsync({ chatId, title }),
				updateStorage,
			};
		},
		{ wrapper },
	).result.current;
	return { queryClient, updateStorage, deps };
};

/** The chat updates sent, keyed by chat id, from the spied API calls. */
const sent = (spy: MockInstance<typeof API.experimental.updateChat>) =>
	Object.fromEntries(spy.mock.calls.map(([chatId, req]) => [chatId, req]));

describe("runPlan", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("executes a plan's label writes and title writes", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const state = stateOf([
			chat("p"),
			chat("m", { "board/group": "p" }),
			chat("s"),
		]);
		const { deps } = renderDeps();

		await expect(
			runPlan(moveCard(state, "p", "Doing", null), deps),
		).resolves.toBe(true);
		await runPlan(renameCard(state, "s", "Solo"), deps);

		expect(sent(spy)).toEqual({
			p: {
				labels: { "board/column": "Doing", "board/pos": expect.any(String) },
			},
			m: { labels: { "board/group": "p", "board/column": "Doing" } },
			s: { title: "Solo" },
		});
		expect(toast).not.toHaveBeenCalled();
	});

	it("applies a plan's storage patch and skips a null plan", async () => {
		const spy = vi.spyOn(API.experimental, "updateChat");
		const state = stateOf([chat("a", { "board/column": "Doing" })]);
		const { deps, updateStorage } = renderDeps();

		await runPlan(addColumn(state, "Review"), deps);
		await runPlan(addColumn(state, "Doing"), deps);

		expect(updateStorage).toHaveBeenCalledTimes(1);
		expect(updateStorage).toHaveBeenCalledWith({
			emptyColumns: ["Review"],
			columnOrder: ["Inbox", "Doing", "Review"],
		});
		expect(spy).not.toHaveBeenCalled();
	});

	it("offers undo that restores the sources first and the receiver after them", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const state = stateOf([
			chat("t", { "board/pos": "200" }),
			chat("s", { "board/pos": "100", "board/title": "Source" }),
		]);
		const { deps } = renderDeps();
		await runPlan(mergeCards(state, "s", "t"), deps);
		expect(toast).toHaveBeenCalledWith(
			'Merged into "Source"',
			expect.objectContaining({ action: expect.anything() }),
		);
		spy.mockClear();
		// The source restore hangs; the receiver's must wait for it.
		const sourceRestore: Deferred<void> = createDeferred();
		spy.mockImplementation((chatId) =>
			chatId === "t" ? sourceRestore.promise : Promise.resolve(),
		);

		lastUndo()();
		await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
		expect(spy).toHaveBeenCalledWith("t", {
			labels: { "board/pos": "200" },
		});
		expect(spy).not.toHaveBeenCalledWith("s", expect.anything());

		sourceRestore.resolve();
		await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
		expect(spy).toHaveBeenLastCalledWith("s", {
			labels: { "board/pos": "100", "board/title": "Source" },
		});
	});

	it("sends the receiver first and skips the sources when it fails", async () => {
		const receiver: Deferred<void> = createDeferred();
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockImplementation((chatId) =>
				chatId === "b" ? receiver.promise : Promise.resolve(),
			);
		const state = stateOf([
			chat("a", { "board/pos": "200", ...addCommentLabels({}, "one", 1) }),
			chat("b", { "board/pos": "100" }),
		]);
		const { queryClient, deps } = renderDeps();

		const pending = runPlan(moveNote(state, "a", 0, "b", null), deps);
		await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
		expect(spy).toHaveBeenCalledWith("b", expect.anything());
		receiver.reject(new Error("too many labels"));

		await expect(pending).resolves.toBe(false);
		// The plan resolves on the receiver's rejection; the queued sources
		// run after it.
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		expect(spy).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith("too many labels");
		expect(toast).not.toHaveBeenCalled();
	});

	it("reports a failed undo of a plan without a receiver with a toast only", async () => {
		// Vitest fails the run on an unhandled rejection, so reaching the
		// assertions is the check that the rejection was observed.
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const state = stateOf([
			chat("a", {
				"board/pos": "200",
				...addCommentLabels(addCommentLabels({}, "one", 1), "two", 2),
			}),
		]);
		const { deps } = renderDeps();
		await runPlan(moveNote(state, "a", 0, "a", null), deps);
		spy.mockRejectedValue(new Error("undo rejected"));

		lastUndo()();

		await waitFor(() =>
			expect(toast.error).toHaveBeenCalledWith("undo rejected"),
		);
	});

	it("reports a failed source write with a toast, resolves false, and offers no undo", async () => {
		// Untitled single "s" onto untitled single "t": "t" keeps and receives,
		// "s" is the source write that fails.
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockImplementation((chatId) =>
				chatId === "s" ? Promise.reject(new Error("boom")) : Promise.resolve(),
			);
		const state = stateOf([chat("t", { "board/pos": "200" }), chat("s")]);
		const { deps } = renderDeps();

		await expect(runPlan(mergeCards(state, "s", "t"), deps)).resolves.toBe(
			false,
		);

		expect(spy).toHaveBeenCalledWith("t", expect.anything());
		expect(toast.error).toHaveBeenCalledWith("boom");
		expect(toast).not.toHaveBeenCalled();
	});

	it("patches the board and sidebar caches before the request settles, then refreshes once quiet", async () => {
		const request: Deferred<void> = createDeferred();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const primary = chat("p", { "board/column": "Inbox" });
		const { queryClient, deps } = renderDeps();
		const listKey = infiniteChats({}).queryKey;
		queryClient.setQueryData(listKey, { pages: [[primary]], pageParams: [0] });
		queryClient.setQueryData(boardChatsKey, {
			pages: [[primary]],
			pageParams: [0],
		});
		const written = {
			"board/column": "Doing",
			"board/pos": expect.any(String),
		};

		const pending = runPlan(
			moveCard(stateOf([primary]), "p", "Doing", null),
			deps,
		);

		await waitFor(() =>
			expect(
				queryClient.getQueryData<{ pages: Chat[][] }>(boardChatsKey)
					?.pages[0]?.[0]?.labels,
			).toEqual(written),
		);
		expect(
			queryClient.getQueryData<{ pages: Chat[][] }>(listKey)?.pages[0]?.[0]
				?.labels,
		).toEqual(written);
		expect(queryClient.getQueryState(boardChatsKey)?.isInvalidated).toBe(false);

		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
		request.resolve();
		await pending;
		expect(queryClient.getQueryState(boardChatsKey)?.isInvalidated).toBe(false);

		vi.runOnlyPendingTimers();
		vi.useRealTimers();
		expect(queryClient.getQueryState(boardChatsKey)?.isInvalidated).toBe(true);
	});
});
