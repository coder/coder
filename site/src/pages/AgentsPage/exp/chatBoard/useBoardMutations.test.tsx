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
import { addCommentLabels, buildCards } from "./boardLabels";
import { useBoardMutations } from "./useBoardMutations";

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
	labels,
});

const renderMutations = () => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	const wrapper = ({ children }: PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return {
		queryClient,
		result: renderHook(() => useBoardMutations(), { wrapper }).result,
	};
};

/** The label maps written, keyed by chat id, from the spied API calls. */
const written = (spy: MockInstance<typeof API.experimental.updateChat>) =>
	Object.fromEntries(
		spy.mock.calls.map(([chatId, req]) => [chatId, req.labels]),
	);

describe("useBoardMutations", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("moveCard sets the column on every member and the position on the primary", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const [card] = buildCards([
			chat("p", { "board/column": "Inbox" }),
			chat("m", { "board/group": "p", "other/x": "1" }),
		]);
		if (!card) throw new Error("card missing");

		await renderMutations().result.current.moveCard(card, "Doing", 4200);

		expect(written(spy)).toEqual({
			p: { "board/column": "Doing", "board/pos": "4200" },
			m: { "board/group": "p", "board/column": "Doing", "other/x": "1" },
		});
	});

	it("mergeCards keeps a group dropped onto a single chat and moves it into the target slot", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const [target, source] = buildCards([
			chat("t", {
				"board/pos": "200",
				"board/column": "Done",
				...addCommentLabels({}, "kept", 1),
			}),
			chat("s", {
				"board/pos": "100",
				"board/title": "Source",
				"board/color": "sky",
				...addCommentLabels({}, "moved", 2),
			}),
			chat("sm", { "board/group": "s" }),
		]);
		if (!target || !source) throw new Error("cards missing");

		await renderMutations().result.current.mergeCards(source, target);

		// The group survives with its title; the lone chat joins it and the
		// group takes over the slot where it was dropped.
		expect(written(spy)).toEqual({
			s: {
				"board/pos": "200",
				"board/column": "Done",
				"board/title": "Source",
				"board/color": "sky",
				"board/comment.0.timestamp": "2",
				"board/comment.0.0": "moved",
				"board/comment.1.timestamp": "1",
				"board/comment.1.0": "kept",
			},
			t: { "board/group": "s", "board/column": "Done" },
			sm: { "board/group": "s", "board/column": "Done" },
		});
	});

	it("mergeCards of two titled groups keeps the target and records the lost title as a note", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		vi.spyOn(Date, "now").mockReturnValue(5);
		const [target, source] = buildCards([
			chat("t", { "board/pos": "200", "board/title": "Target" }),
			chat("tm", { "board/group": "t" }),
			chat("s", { "board/pos": "100", "board/title": "Source" }),
			chat("sm", { "board/group": "s" }),
		]);
		if (!target || !source) throw new Error("cards missing");

		await renderMutations().result.current.mergeCards(source, target);

		expect(written(spy)).toEqual({
			t: {
				"board/pos": "200",
				"board/title": "Target",
				"board/comment.0.timestamp": "5",
				"board/comment.0.0": "Merged card: Source",
			},
			s: { "board/group": "t" },
			sm: { "board/group": "t" },
		});
	});

	it("undo puts every touched chat's labels back", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const [target, source] = buildCards([
			chat("t", { "board/pos": "200" }),
			chat("s", { "board/pos": "100", "board/title": "Source" }),
		]);
		if (!target || !source) throw new Error("cards missing");
		const { result } = renderMutations();
		await result.current.mergeCards(source, target);
		spy.mockClear();

		lastUndo()();
		await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));

		expect(written(spy)).toEqual({
			s: { "board/pos": "100", "board/title": "Source" },
			t: { "board/pos": "200" },
		});
	});

	it("detachChat makes the chat its own card in the target column", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const [card] = buildCards([chat("p"), chat("m", { "board/group": "p" })]);
		if (!card) throw new Error("card missing");
		const member = card.members[1];
		if (!member) throw new Error("member missing");

		await renderMutations().result.current.detachChat(member, card, "Later", 7);

		expect(written(spy)).toEqual({
			m: { "board/column": "Later", "board/pos": "7" },
		});
	});

	it("detachChat of the primary hands the card to the next member without renaming it", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		const [card] = buildCards([
			{
				...chat("p", {
					"board/color": "sky",
					"board/pos": "300",
					"board/column": "Doing",
					...addCommentLabels({}, "note", 1),
				}),
				title: "Primary chat title",
			},
			chat("a", { "board/group": "p", "board/column": "Doing" }),
			chat("b", { "board/group": "p", "board/column": "Doing" }),
		]);
		if (!card) throw new Error("card missing");

		await renderMutations().result.current.detachChat(
			card.primary,
			card,
			"Doing",
			299,
		);

		expect(written(spy)).toEqual({
			a: {
				"board/column": "Doing",
				"board/color": "sky",
				"board/pos": "300",
				"board/comment.0.timestamp": "1",
				"board/comment.0.0": "note",
				"board/title": "Primary chat title",
			},
			b: { "board/group": "a", "board/column": "Doing" },
			p: { "board/column": "Doing", "board/pos": "299" },
		});
	});

	it("addComment appends a note after the existing ones", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		vi.spyOn(Date, "now").mockReturnValue(5000);
		const [card] = buildCards([chat("p", addCommentLabels({}, "first", 1))]);
		if (!card) throw new Error("card missing");

		await renderMutations().result.current.addComment(card, "second");

		expect(written(spy).p).toEqual({
			"board/comment.0.timestamp": "1",
			"board/comment.0.0": "first",
			"board/comment.1.timestamp": "5000",
			"board/comment.1.0": "second",
		});
	});

	it("reports a failed write with a toast and resolves", async () => {
		const { toast } = await import("sonner");
		vi.spyOn(API.experimental, "updateChat").mockRejectedValue(
			new Error("boom"),
		);
		const [card] = buildCards([chat("p")]);
		if (!card) throw new Error("card missing");

		await expect(
			renderMutations().result.current.setCardTitle(card, "New"),
		).resolves.toBeUndefined();
		expect(toast.error).toHaveBeenCalledWith("boom");
	});

	it("patches the cached chat list before the request settles, then invalidates it", async () => {
		const request = createDeferred<void>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(request.promise);
		const primary = chat("p", { "board/column": "Inbox" });
		const [card] = buildCards([primary]);
		if (!card) throw new Error("card missing");
		const { queryClient, result } = renderMutations();
		const listKey = infiniteChats({}).queryKey;
		queryClient.setQueryData(listKey, { pages: [[primary]], pageParams: [0] });
		const cachedLabels = () =>
			queryClient.getQueryData<{ pages: Chat[][] }>(listKey)?.pages[0]?.[0]
				?.labels;

		const pending = result.current.moveCard(card, "Doing", 4200);

		await waitFor(() =>
			expect(cachedLabels()).toEqual({
				"board/column": "Doing",
				"board/pos": "4200",
			}),
		);
		expect(queryClient.getQueryState(listKey)?.isInvalidated).toBe(false);

		request.resolve();
		await pending;

		expect(queryClient.getQueryState(listKey)?.isInvalidated).toBe(true);
	});
});
