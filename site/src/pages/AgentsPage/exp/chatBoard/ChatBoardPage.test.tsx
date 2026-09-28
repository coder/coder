import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as Sonner from "sonner";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import ChatBoardPage from "./ChatBoardPage";

// The page shell mounts the Toaster; only the toast calls are replaced.
vi.mock("sonner", async (importOriginal) => ({
	...(await importOriginal<typeof Sonner>()),
	toast: Object.assign(vi.fn(), { error: vi.fn() }),
}));

const isSearch = (req: { q?: string } | undefined) =>
	req?.q?.includes("search:") ?? false;

// The list and the filter share one endpoint; the filter is the call that
// carries a search token.
const mockChats = (
	list: () => Promise<Chat[]>,
	search: () => Promise<Chat[]>,
) =>
	vi
		.spyOn(API.experimental, "getChats")
		.mockImplementation((req) => (isSearch(req) ? search() : list()));

describe("ChatBoardPage", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
	});

	it("saves columns first seen in the labels at the end of the column order", async () => {
		const inColumn = (id: string, column: string, pos: string): Chat => ({
			...MockChat,
			id,
			labels: { "board/column": column, "board/pos": pos },
		});
		mockChats(
			() =>
				Promise.resolve([
					inColumn("a", "Review", "300"),
					inColumn("b", "Doing", "200"),
				]),
			() => Promise.resolve([]),
		);
		renderWithAuth(<ChatBoardPage />);
		await screen.findAllByRole("article");

		const key = Object.keys(localStorage).find((k) =>
			k.startsWith("agents.board."),
		);
		expect(
			JSON.parse(localStorage.getItem(key ?? "") ?? "{}").columnOrder,
		).toEqual(["Inbox", "Review", "Doing"]);
	});

	it("renaming a column with cards leaves no column under the old name", async () => {
		const user = userEvent.setup();
		mockChats(
			() =>
				Promise.resolve([
					{ ...MockChat, labels: { "board/column": "Review" } },
				]),
			() => Promise.resolve([]),
		);
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		await user.click(screen.getByRole("button", { name: "Review" }));
		const input = screen.getByRole("textbox", { name: "Review column name" });
		await user.clear(input);
		await user.type(input, "Done{Enter}");

		await screen.findByRole("region", { name: "Done column" });
		expect(
			screen.queryByRole("region", { name: "Review column" }),
		).not.toBeInTheDocument();
	});

	it("shows the search error instead of an unfiltered board", async () => {
		const user = userEvent.setup();
		mockChats(
			() => Promise.resolve([MockChat]),
			() => Promise.reject({ message: "Search is down" }),
		);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		await user.type(screen.getByRole("textbox", { name: "Filter cards" }), "x");

		await screen.findByText("Search is down");
		expect(screen.queryAllByRole("article")).toHaveLength(0);
	});

	it("shows a loader, not counts or cards, before the list resolves", async () => {
		mockChats(
			() => new Promise(() => {}),
			() => Promise.resolve([]),
		);
		renderWithAuth(<ChatBoardPage />);

		// The auth wrapper shows its own loader first; the filter box proves the
		// board is what is mounted.
		await screen.findByRole("textbox", { name: "Filter cards" });
		screen.getByRole("status", { name: "Loading" });
		expect(screen.queryByText(/cards$/)).toBeNull();
		expect(screen.queryAllByRole("article")).toHaveLength(0);
	});

	it("keeps the text of a note whose write failed in a toast", async () => {
		const user = userEvent.setup();
		mockChats(
			() => Promise.resolve([MockChat]),
			() => Promise.resolve([]),
		);
		vi.spyOn(API.experimental, "updateChat").mockRejectedValue(
			new Error("label limit"),
		);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		await user.type(
			screen.getByRole("textbox", { name: `Add a note to ${MockChat.title}` }),
			"remember this{Enter}",
		);

		await vi.waitFor(() =>
			expect(toast).toHaveBeenCalledWith("Note not saved", {
				description: "remember this",
				duration: Number.POSITIVE_INFINITY,
				closeButton: true,
			}),
		);
		expect(toast.error).toHaveBeenCalledWith("label limit");
	});
});
