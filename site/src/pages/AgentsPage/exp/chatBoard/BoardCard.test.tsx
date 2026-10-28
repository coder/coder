import { DragDropProvider } from "@dnd-kit/react";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { BoardCard } from "./BoardCard";
import { dragSensors } from "./boardDrag";
import { buildCards } from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const renderCard = (chats: readonly Chat[], assistant?: Chat) => {
	const [card] = buildCards(chats);
	if (!card) throw new Error("card missing");
	const handlers = {
		onSetTitle: vi.fn(),
		onSetColor: vi.fn(),
		onRenameChat: vi.fn(),
		onCardAssistant: vi.fn(),
		onNewChat: vi.fn(),
		onSetEfforts: vi.fn(),
		onFilterEffort: vi.fn(),
		onRemoveFromGroup: vi.fn(),
		onOpen: vi.fn(),
		onPreview: vi.fn(),
		onPreviewEnd: vi.fn(),
		onAddNote: vi.fn(),
		onEditNote: vi.fn(),
		onRemoveNote: vi.fn(),
		onArchive: vi.fn(),
		onArchiveChat: vi.fn(),
	} satisfies Partial<React.ComponentProps<typeof BoardCard>>;
	// The board's sensors: dnd-kit's defaults have no activation distance, so
	// a pointerdown on the header would start a drag and swallow the click.
	renderComponent(
		<DragDropProvider sensors={dragSensors}>
			<BoardCard
				lane="board"
				card={card}
				assistant={assistant}
				openChatIds={new Set()}
				isDropTarget={false}
				noteDrop={undefined}
				knownEfforts={["Q3", "This week"]}
				{...handlers}
			/>
		</DragDropProvider>,
	);
	return { card, ...handlers };
};

describe("BoardCard", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});
	it("reports an edited title as the card title", async () => {
		const user = userEvent.setup();
		const { onRenameChat, onSetTitle } = renderCard([chat("p")]);

		await user.click(screen.getByRole("button", { name: "Chat p" }));
		const field = screen.getByRole("textbox", { name: "title" });
		await user.clear(field);
		await user.type(field, "Renamed{Enter}");

		// Whether this renames the chat or sets a label is the board's call.
		expect(onSetTitle).toHaveBeenCalledWith("Renamed");
		expect(onRenameChat).not.toHaveBeenCalled();
	});

	it("picks a color from the stripe swatches", async () => {
		const user = userEvent.setup();
		const { onSetColor } = renderCard([chat("p", { "board/color": "red" })]);

		await user.click(screen.getByRole("button", { name: "Color of Chat p" }));
		await user.click(screen.getByRole("button", { name: "sky" }));
		expect(onSetColor).toHaveBeenCalledWith("sky");

		await user.click(screen.getByRole("button", { name: "Color of Chat p" }));
		await user.click(screen.getByRole("button", { name: "No color" }));
		expect(onSetColor).toHaveBeenCalledWith(undefined);
	});

	it("opens the assistant from the actions menu", async () => {
		const user = userEvent.setup();
		const { onCardAssistant } = renderCard([chat("p")]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Assistant" }),
		);

		expect(onCardAssistant).toHaveBeenCalledTimes(1);
	});

	it("archives the card from its menu, and a group row's chat from its own, unless a chat works", async () => {
		const user = userEvent.setup();
		const { onArchive, onArchiveChat } = renderCard([
			chat("p", { "board/title": "Epic" }),
			{ ...chat("m", { "board/group": "p" }), status: "running" },
		]);

		await user.click(screen.getByRole("button", { name: "Actions for Epic" }));
		expect(
			await screen.findByRole("menuitem", {
				name: "Archive card (a chat is working)",
			}),
		).toHaveAttribute("aria-disabled", "true");
		await user.keyboard("{Escape}");

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Archive chat" }),
		);
		expect(onArchiveChat).toHaveBeenCalledWith(
			expect.objectContaining({ id: "p" }),
		);
		expect(onArchive).not.toHaveBeenCalled();
	});

	it("starts a new chat in the card from the actions menu", async () => {
		const user = userEvent.setup();
		const { onNewChat } = renderCard([chat("p")]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "New chat in card" }),
		);

		expect(onNewChat).toHaveBeenCalledTimes(1);
	});

	it("edits its efforts from the menu, toggling known ones and coining new ones", async () => {
		const user = userEvent.setup();
		const { onSetEfforts } = renderCard([
			chat("p", { "board/effort.0": "Q3" }),
		]);

		await user.click(
			screen.getByRole("button", { name: "Actions for Chat p" }),
		);
		// Opened from the keyboard: without layout, jsdom cannot tell a pointer
		// heading into the sub menu from one leaving it, and would close it.
		(await screen.findByRole("menuitem", { name: "Efforts" })).focus();
		await user.keyboard("{ArrowRight}");
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "This week" }),
		);
		expect(onSetEfforts).toHaveBeenCalledWith(["Q3", "This week"]);

		// The sub menu stayed open.
		await user.click(screen.getByRole("menuitemcheckbox", { name: "Q3" }));
		expect(onSetEfforts).toHaveBeenLastCalledWith([]);

		// Blur must not save the partial name.
		const field = screen.getByRole("textbox", { name: "New effort" });
		await user.type(field, "Lau");
		await user.hover(screen.getByRole("menuitemcheckbox", { name: "Q3" }));
		expect(field).not.toHaveFocus();
		await user.type(field, "nch{Enter}");
		expect(onSetEfforts).toHaveBeenCalledTimes(3);
		expect(onSetEfforts).toHaveBeenLastCalledWith(["Q3", "Launch"]);
		expect(field).toHaveValue("");
		await user.type(field, "Beta{Enter}");
		expect(onSetEfforts).toHaveBeenLastCalledWith(["Q3", "Beta"]);
	});

	it("filters the board by an effort from its tag", async () => {
		const user = userEvent.setup();
		const { onFilterEffort } = renderCard([
			chat("p", { "board/effort.0": "Q3" }),
		]);

		await user.click(screen.getByRole("button", { name: "Filter by Q3" }));

		expect(onFilterEffort).toHaveBeenCalledWith("Q3");
	});

	it("opens the chat from its icon with the card as anchor", async () => {
		const user = userEvent.setup();
		const { card, onOpen } = renderCard([chat("p")]);

		// The header surface shares the icon's label; the icon alone has the title.
		await user.click(screen.getByTitle("Open chat"));

		expect(onOpen).toHaveBeenCalledWith(
			card.primary,
			expect.objectContaining({ top: expect.any(Number) }),
		);
	});

	it("anchors a group row's window to that row, not the card", async () => {
		const user = userEvent.setup();
		const { card, onOpen, onPreview } = renderCard([
			chat("p", { "board/group": "p" }),
			chat("m", { "board/group": "p" }),
		]);
		const rect = (top: number) => new DOMRect(0, top, 0, 0);
		vi.spyOn(
			screen.getByRole("article"),
			"getBoundingClientRect",
		).mockReturnValue(rect(10));
		const [, secondRow] = screen.getAllByRole("listitem");
		vi.spyOn(secondRow, "getBoundingClientRect").mockReturnValue(rect(120));

		const opener = screen.getAllByTitle("Open chat")[1];
		await user.hover(opener);
		await user.click(opener);

		expect(onPreview).toHaveBeenCalledWith(card.members[1], rect(120));
		expect(onOpen).toHaveBeenCalledWith(card.members[1], rect(120));
	});

	it("previews and opens the assistant from its icon, anchored to the card", async () => {
		const user = userEvent.setup();
		const assistant = chat("a", { "board/assistant": "p" });
		const { onOpen, onPreview } = renderCard([chat("p")], assistant);
		const rect = new DOMRect(0, 10, 0, 0);
		vi.spyOn(
			screen.getByRole("article"),
			"getBoundingClientRect",
		).mockReturnValue(rect);

		const opener = screen.getByRole("button", { name: "Assistant" });
		await user.hover(opener);
		await user.click(opener);

		expect(onPreview).toHaveBeenCalledWith(assistant, rect);
		expect(onOpen).toHaveBeenCalledWith(assistant, rect);
	});
});
