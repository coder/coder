import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { toast } from "sonner";
import { afterEach, describe, expect, it, type MockInstance, vi } from "vitest";
import { API } from "#/api/api";
import { infiniteChats } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import type { BoardState } from "./boardApi";
import { buildCards, buildColumns } from "./boardLabels";
import type { BoardStorage } from "./boardStorage";
import { useBoardApi } from "./useBoardApi";

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

const renderApi = (state: BoardState) => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	const updateStorage = vi.fn();
	return {
		queryClient,
		updateStorage,
		board: renderHook(() => useBoardApi(state, updateStorage), { wrapper })
			.result.current,
	};
};

/** The chat updates sent, keyed by chat id, from the spied API calls. */
const sent = (spy: MockInstance<typeof API.experimental.updateChat>) =>
	Object.fromEntries(spy.mock.calls.map(([chatId, req]) => [chatId, req]));

describe("useBoardApi", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("executes a plan's label writes and title writes", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const { board } = renderApi(
			stateOf([chat("p"), chat("m", { "board/group": "p" }), chat("s")]),
		);

		await board.moveCard("p", "Doing", null);
		await board.renameCard("s", "Solo");

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
		const { board, updateStorage } = renderApi(
			stateOf([chat("a", { "board/column": "Doing" })]),
		);

		await board.addColumn("Review");
		await board.addColumn("Doing");

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
		const { board } = renderApi(
			stateOf([
				chat("t", { "board/pos": "200" }),
				chat("s", { "board/pos": "100", "board/title": "Source" }),
			]),
		);
		await board.mergeCards("s", "t");
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
		const { board } = renderApi(
			stateOf([chat("t", { "board/pos": "200" }), chat("s")]),
		);

		await expect(board.mergeCards("s", "t")).resolves.toBeUndefined();

		expect(toast.error).toHaveBeenCalledWith("boom");
		expect(toast).not.toHaveBeenCalled();
	});

	it("patches the cached chat list before the request settles, then invalidates it", async () => {
		const request = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const primary = chat("p", { "board/column": "Inbox" });
		const { queryClient, board } = renderApi(stateOf([primary]));
		const listKey = infiniteChats({}).queryKey;
		queryClient.setQueryData(listKey, { pages: [[primary]], pageParams: [0] });
		const cachedLabels = () =>
			queryClient.getQueryData<{ pages: Chat[][] }>(listKey)?.pages[0]?.[0]
				?.labels;

		const pending = board.moveCard("p", "Doing", null);

		await waitFor(() =>
			expect(cachedLabels()).toEqual({
				"board/column": "Doing",
				"board/pos": expect.any(String),
			}),
		);
		expect(queryClient.getQueryState(listKey)?.isInvalidated).toBe(false);

		request.resolve();
		await pending;

		expect(queryClient.getQueryState(listKey)?.isInvalidated).toBe(true);
	});
});
