import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as Sonner from "sonner";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat, ChatStatus } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import type { CreateChatOptions } from "../../components/AgentCreateForm";
import ChatBoardPage from "./ChatBoardPage";

// The page shell mounts the Toaster; only the toast calls are replaced.
vi.mock("sonner", async (importOriginal) => ({
	...(await importOriginal<typeof Sonner>()),
	toast: Object.assign(vi.fn(), { error: vi.fn() }),
}));

// Windows are under test, not the chat page or the create form's loading.
vi.mock("../../AgentChatPage", () => ({
	default: ({ chatId }: { chatId: string }) => <p>chat {chatId}</p>,
}));
vi.mock("../../components/AgentCreateForm", () => ({
	AgentCreateForm: ({
		onCreateChat,
	}: {
		onCreateChat: (options: CreateChatOptions) => Promise<void>;
	}) => (
		<button
			type="button"
			onClick={() =>
				void onCreateChat({ message: "Hello", organizationId: "org-1" })
			}
		>
			send
		</button>
	),
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

/** Each list fetch answers with the next response; the last one repeats. */
const listResponses = (...responses: Chat[][]) => {
	let calls = 0;
	return () =>
		Promise.resolve(responses[Math.min(calls++, responses.length - 1)]);
};

// Watch events would change a chat's status; a focus refetch stands in.
const refetchOnFocus = () =>
	window.dispatchEvent(new Event("visibilitychange"));

const launch = (labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id: "launch",
	title: "Launch",
	labels,
});

const openCardDraft = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		await screen.findByRole("button", { name: "Actions for Launch" }),
	);
	await user.click(
		await screen.findByRole("menuitem", { name: "New chat in card" }),
	);
	await screen.findByText("New chat in Launch");
};

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

	it("renaming the selected effort keeps it selected", async () => {
		const user = userEvent.setup();
		mockChats(
			() =>
				Promise.resolve([{ ...MockChat, labels: { "board/effort.0": "Q3" } }]),
			() => Promise.resolve([]),
		);
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		await user.click(screen.getByRole("button", { name: "Efforts" }));
		await user.click(await screen.findByRole("menuitemradio", { name: /Q3/ }));
		await user.click(screen.getByRole("button", { name: "Q3" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "Rename effort" }),
		);
		const input = screen.getByRole("textbox", { name: "Effort name" });
		await user.clear(input);
		await user.type(input, "Q4{Enter}");

		expect(
			await screen.findByRole("button", { name: "Q4" }),
		).toBeInTheDocument();
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

	it("shows the board assistant's label writes once its turn ends", async () => {
		const user = userEvent.setup();
		const assistant = (status: ChatStatus): Chat => ({
			...MockChat,
			id: "board-assistant",
			status,
			labels: { "board/assistant": "board" },
		});
		mockChats(
			listResponses(
				[launch(), assistant("running")],
				[launch(), assistant("waiting")],
				[launch({ "board/column": "Done" }), assistant("waiting")],
			),
			() => Promise.resolve([]),
		);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		await user.click(screen.getByRole("button", { name: "Board assistant" }));
		await screen.findByText("chat board-assistant");

		// Only the status changes here; the column arrives with the refetch
		// the turn's end triggers.
		refetchOnFocus();
		expect(
			await screen.findByRole("region", { name: "Done column" }),
		).toBeInTheDocument();
	});

	it("clears a saved effort filter that no card carries", async () => {
		const key = `agents.board.${MockUserOwner.id}`;
		localStorage.setItem(key, JSON.stringify({ effortFilter: "Gone" }));
		mockChats(
			() => Promise.resolve([launch()]),
			() => Promise.resolve([]),
		);
		renderWithAuth(<ChatBoardPage />);
		await screen.findByRole("article");

		expect(JSON.parse(localStorage.getItem(key) ?? "{}").effortFilter).toBe(
			null,
		);
	});

	it("closes a card's draft when the card is gone", async () => {
		const user = userEvent.setup();
		mockChats(listResponses([launch()], [{ ...MockChat, id: "other" }]), () =>
			Promise.resolve([]),
		);
		renderWithAuth(<ChatBoardPage />);
		await openCardDraft(user);

		refetchOnFocus();
		// Hidden alone, the draft would come back with a reload; it must be
		// dropped from the saved windows too.
		await waitFor(() => {
			const key = Object.keys(localStorage).find((k) =>
				k.startsWith("agents.board."),
			);
			expect(
				JSON.parse(localStorage.getItem(key ?? "") ?? "{}").windows,
			).toEqual([]);
		});
		expect(screen.queryByText("New chat in Launch")).toBeNull();
	});

	it("creates a card's chat with the card context and swaps the draft for the chat", async () => {
		const user = userEvent.setup();
		mockChats(
			() => Promise.resolve([launch({ "board/column": "Doing" })]),
			() => Promise.resolve([]),
		);
		const create = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-chat" });
		renderWithAuth(<ChatBoardPage />);
		await openCardDraft(user);

		await user.click(
			screen.getByRole("checkbox", { name: "Include card context" }),
		);
		await user.click(screen.getByRole("button", { name: "send" }));

		await screen.findByText("chat new-chat");
		expect(screen.queryByText("New chat in Launch")).toBeNull();
		expect(create).toHaveBeenCalledWith(
			expect.objectContaining({
				labels: { "board/group": "launch", "board/column": "Doing" },
				content: [
					{
						type: "text",
						text: expect.stringContaining("Card context\nTitle: Launch"),
					},
				],
			}),
		);
	});
});
