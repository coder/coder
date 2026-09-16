import { renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, type MockInstance, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { addCommentLabels, buildCards } from "./boardLabels";
import { useBoardMutations } from "./useBoardMutations";

vi.mock("sonner", () => ({
	toast: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
}));

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
	return renderHook(() => useBoardMutations(), { wrapper }).result;
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

		await renderMutations().current.moveCard(card, "Doing", 4200);

		expect(written(spy)).toEqual({
			p: { "board/column": "Doing", "board/pos": "4200" },
			m: { "board/group": "p", "board/column": "Doing", "other/x": "1" },
		});
	});

	it("mergeCards regroups the source members and carries its notes over", async () => {
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

		await renderMutations().current.mergeCards(source, target);

		expect(written(spy)).toEqual({
			s: { "board/group": "t", "board/column": "Done" },
			sm: { "board/group": "t", "board/column": "Done" },
			t: {
				"board/pos": "200",
				"board/column": "Done",
				"board/comment.0.timestamp": "1",
				"board/comment.0.0": "kept",
				"board/comment.1.timestamp": "2",
				"board/comment.1.0": "moved",
			},
		});
	});

	it("detachChat makes the chat its own card in the target column", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);

		await renderMutations().current.detachChat(
			chat("m", { "board/group": "p" }),
			"Later",
			7,
		);

		expect(written(spy)).toEqual({
			m: { "board/column": "Later", "board/pos": "7" },
		});
	});

	it("addComment appends a note after the existing ones", async () => {
		const spy = vi
			.spyOn(API.experimental, "updateChat")
			.mockResolvedValue(undefined);
		vi.spyOn(Date, "now").mockReturnValue(5000);
		const [card] = buildCards([chat("p", addCommentLabels({}, "first", 1))]);
		if (!card) throw new Error("card missing");

		await renderMutations().current.addComment(card, "second");

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
			renderMutations().current.setCardTitle(card, "New"),
		).resolves.toBeUndefined();
		expect(toast.error).toHaveBeenCalledWith("boom");
	});
});
