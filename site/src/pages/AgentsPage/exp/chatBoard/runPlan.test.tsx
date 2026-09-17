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
import { createDeferred } from "#/testHelpers/deferred";
import {
	addColumn,
	type BoardState,
	mergeCards,
	moveCard,
	renameCard,
} from "./boardApi";
import { boardChatsKey, updateChatLabels } from "./boardChats";
import { buildCards, buildColumns } from "./boardLabels";
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

// The page wires the two mutations the same way; the test runs them for
// real so the cache behaviour of updateChatLabels is covered.
const usePlanDeps = (updateStorage: PlanDeps["updateStorage"]): PlanDeps => {
	const queryClient = useQueryClient();
	const labels = useMutation({
		...updateChatLabels(queryClient),
		onError: (error: Error) => toast.error(error.message),
	});
	const titles = useMutation(updateChatTitle(queryClient));
	return {
		write: (chatId, map) => labels.mutateAsync({ chatId, labels: map }),
		rename: (chatId, title) => titles.mutateAsync({ chatId, title }),
		updateStorage,
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
	const deps = renderHook(() => usePlanDeps(updateStorage), { wrapper }).result
		.current;
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

		await runPlan(moveCard(state, "p", "Doing", null), deps);
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

	it("offers undo that puts every touched chat's labels back", async () => {
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

		lastUndo()();
		await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));

		expect(sent(spy)).toEqual({
			s: { labels: { "board/pos": "100", "board/title": "Source" } },
			t: { labels: { "board/pos": "200" } },
		});
	});

	it("reports a failed write with a toast, resolves, and offers no undo", async () => {
		vi.spyOn(API.experimental, "updateChat").mockImplementation((chatId) =>
			chatId === "t" ? Promise.reject(new Error("boom")) : Promise.resolve(),
		);
		const state = stateOf([chat("t", { "board/pos": "200" }), chat("s")]);
		const { deps } = renderDeps();

		await expect(runPlan(mergeCards(state, "s", "t"), deps)).resolves.toBe(
			undefined,
		);

		expect(toast.error).toHaveBeenCalledWith("boom");
		expect(toast).not.toHaveBeenCalled();
	});

	it("patches the board and sidebar caches before the request settles, then invalidates", async () => {
		const request = createDeferred<void>();
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

		request.resolve();
		await pending;

		expect(queryClient.getQueryState(boardChatsKey)?.isInvalidated).toBe(true);
	});

	it("keeps the patch when a board refetch started before the write lands after it", async () => {
		const request = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const primary = chat("p");
		const { queryClient, deps } = renderDeps();
		queryClient.setQueryData(boardChatsKey, {
			pages: [[primary]],
			pageParams: [0],
		});
		const staleFetch = createDeferred<Chat[]>();
		const refetch = queryClient.fetchInfiniteQuery({
			queryKey: boardChatsKey,
			queryFn: () => staleFetch.promise,
			initialPageParam: 0,
			staleTime: 0,
		});
		const cachedLabels = () =>
			queryClient.getQueryData<{ pages: Chat[][] }>(boardChatsKey)
				?.pages[0]?.[0]?.labels;
		const written = {
			"board/column": "Doing",
			"board/pos": expect.any(String),
		};

		const pending = runPlan(
			moveCard(stateOf([primary]), "p", "Doing", null),
			deps,
		);
		await waitFor(() => expect(cachedLabels()).toEqual(written));

		// The server answers the old refetch with pre-write labels.
		staleFetch.resolve([primary]);
		await refetch.catch(() => undefined);
		request.resolve();
		await pending;

		expect(cachedLabels()).toEqual(written);
	});
});
